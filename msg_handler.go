/**
* @File: msg_handler.go
* @Author: Jason Woo
* @Date: 2023/8/17 13:20
**/

package xnet

import (
	"encoding/hex"
	"github.com/dyowoo/xnet/xlog"
	"sync"
)

type IMsgHandle interface {
	AddRouter(uint32, ...RouterHandler) IRouter
	Use(...RouterHandler) IRouter
	UseGateway(RouterHandler)
	StartWorkerPool()
	SendMsgToTaskQueue(IRequest) // 将消息交给TaskQueue,由worker进行处理
	Execute(IRequest)            // 执行责任链上的拦截器方法
	AddInterceptor(IInterceptor) // 注册责任链任务入口，每个拦截器处理完后，数据都会传递至下一个拦截器，使得消息可以层层处理层层传递，顺序取决于注册顺序
}

const (
	// WorkerIDWithoutWorkerPool (如果不启动Worker协程池，则会给MsgHandler分配一个虚拟的WorkerID，这个workerID为0, 便于指标统计
	WorkerIDWithoutWorkerPool int = 0
)

// MsgHandle 对消息的处理回调模块
type MsgHandle struct {
	gatewayHandler RouterHandler
	router         *Router
	freeWorkerMu   sync.Mutex
	workerPoolSize uint32              // 业务工作Worker池的数量
	freeWorkers    map[uint32]struct{} // 空闲worker集合
	taskQueue      []chan IRequest     // Worker负责取任务的消息队列
	builder        *chainBuilder       // 责任链构造器
}

func newMsgHandle() *MsgHandle {
	var freeWorker map[uint32]struct{}

	if GlobalConfig.WorkerMode == WorkerModeBind {
		GlobalConfig.WorkerPoolSize = uint32(GlobalConfig.MaxConn)
		freeWorker = make(map[uint32]struct{}, GlobalConfig.WorkerPoolSize)

		for i := uint32(0); i < GlobalConfig.WorkerPoolSize; i++ {
			freeWorker[i] = struct{}{}
		}
	}

	msgHandle := &MsgHandle{
		gatewayHandler: nil,
		router:         NewRouter(),
		workerPoolSize: GlobalConfig.WorkerPoolSize,
		taskQueue:      make([]chan IRequest, GlobalConfig.WorkerPoolSize),
		freeWorkers:    freeWorker,
		builder:        newChainBuilder(),
	}

	msgHandle.builder.Tail(msgHandle)

	return msgHandle
}

// Use worker ID
// 占用workerID
func useWorker(conn IConnection) uint32 {
	mh, _ := conn.GetMsgHandler().(*MsgHandle)
	if mh == nil {
		xlog.ErrorF("useWorker failed, mh is nil")
		return 0
	}

	if GlobalConfig.WorkerMode == WorkerModeBind {
		mh.freeWorkerMu.Lock()
		defer mh.freeWorkerMu.Unlock()

		for k := range mh.freeWorkers {
			delete(mh.freeWorkers, k)
			return k
		}
	}

	if mh.workerPoolSize <= 0 {
		return 0
	}

	// 根据ConnID来分配当前的连接应该由哪个worker负责处理
	// 轮询的平均分配法则
	// 得到需要处理此条连接的workerID
	return uint32(conn.GetConnID() % uint64(mh.workerPoolSize))
}

// 释放workerID
func freeWorker(conn IConnection) {
	mh, _ := conn.GetMsgHandler().(*MsgHandle)
	if mh == nil {
		xlog.ErrorF("useWorker failed, mh is nil")
		return
	}

	if GlobalConfig.WorkerMode == WorkerModeBind {
		mh.freeWorkerMu.Lock()
		defer mh.freeWorkerMu.Unlock()

		mh.freeWorkers[conn.GetWorkerID()] = struct{}{}
	}
}

// Intercept 默认必经的数据处理拦截器
func (mh *MsgHandle) Intercept(chain IChain) IcResp {
	request := chain.Request()

	if request != nil {
		switch request.(type) {
		case IRequest:
			iRequest := request.(IRequest)

			if GlobalConfig.WorkerPoolSize > 0 {
				// 已经启动工作池机制，将消息交给Worker处理
				mh.SendMsgToTaskQueue(iRequest)
			} else {
				// 从绑定好的消息和对应的处理方法中执行对应的Handle方法
				go mh.doMsgHandler(iRequest, WorkerIDWithoutWorkerPool)
			}
		}
	}

	return chain.Proceed(chain.Request())
}

func (mh *MsgHandle) AddInterceptor(interceptor IInterceptor) {
	if mh.builder != nil {
		mh.builder.AddInterceptor(interceptor)
	}
}

// SendMsgToTaskQueue 将消息交给TaskQueue,由worker进行处理
func (mh *MsgHandle) SendMsgToTaskQueue(request IRequest) {
	workerID := request.GetConnection().GetWorkerID()
	mh.taskQueue[workerID] <- request
	xlog.DebugF("sendMsgToTaskQueue-->%s", hex.EncodeToString(request.GetData()))
}

// doFuncHandler 执行函数式请求
func (mh *MsgHandle) doFuncHandler(request IFuncRequest, workerID int) {
	defer func() {
		if err := recover(); err != nil {
			xlog.ErrorF("workerID: %d doFuncRequest panic: %v", workerID, err)
		}
	}()

	request.CallFunc()
}

func (mh *MsgHandle) Execute(request IRequest) {
	// 将消息丢到责任链，通过责任链里拦截器层层处理层层传递
	mh.builder.Execute(request)
}

// AddRouter 路由添加
func (mh *MsgHandle) AddRouter(msgId uint32, handler ...RouterHandler) IRouter {
	mh.router.AddHandler(msgId, handler...)
	return mh.router
}

func (mh *MsgHandle) Use(handlers ...RouterHandler) IRouter {
	mh.router.Use(handlers...)
	return mh.router
}

func (mh *MsgHandle) UseGateway(handler RouterHandler) {
	mh.gatewayHandler = handler
}

func (mh *MsgHandle) doMsgHandler(request IRequest, workerID int) {
	defer func() {
		if err := recover(); err != nil {
			xlog.ErrorF("workerID: %d doMsgHandler panic: %v", workerID, err)
		}
	}()

	if mh.gatewayHandler != nil {
		mh.gatewayHandler(request)
		return
	}

	msgId := request.GetMsgID()
	var handlers []RouterHandler
	handlers, ok := mh.router.GetHandlers(msgId)
	if !ok {
		xlog.ErrorF("api msgID = %d is not FOUND!", msgId)
		return
	}

	request.BindRouter(handlers)
	request.RouterNext()
}

// StartOneWorker 启动一个Worker工作流程
func (mh *MsgHandle) StartOneWorker(workerID int, taskQueue chan IRequest) {
	xlog.InfoF("Worker ID = %d is started.", workerID)

	// 不断地等待队列中的消息
	for {
		select {
		// 有消息则取出队列的Request，并执行绑定的业务方法
		case request := <-taskQueue:
			switch req := request.(type) {
			case IFuncRequest:
				// 内部函数调用request
				mh.doFuncHandler(req, workerID)
			case IRequest:
				mh.doMsgHandler(req, workerID)
			}
		}
	}
}

// StartWorkerPool starts the worker pool
func (mh *MsgHandle) StartWorkerPool() {
	// 遍历需要启动worker的数量，依此启动
	for i := 0; i < int(mh.workerPoolSize); i++ {
		// 给当前worker对应的任务队列开辟空间
		mh.taskQueue[i] = make(chan IRequest, GlobalConfig.MaxWorkerTaskLen)

		// 启动当前Worker，阻塞的等待对应的任务队列是否有消息传递进来
		go mh.StartOneWorker(i, mh.taskQueue[i])
	}
}
