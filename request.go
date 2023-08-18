/**
* @File: request.go
* @Author: Jason Woo
* @Date: 2023/8/17 13:25
**/

package xnet

import "sync"

type IFuncRequest interface {
	CallFunc()
}

type IRequest interface {
	GetConnection() IConnection // 获取请求连接信息
	GetData() []byte            // 获取请求消息的数据
	GetMsgID() uint32           // 获取请求的消息ID
	GetMessage() IMessage       // 获取请求消息的原始数据
	GetResponse() IcResp        // 获取解析完后序列化数据
	SetResponse(IcResp)         // 设置解析完后序列化数据
	Abort()                     // 终止处理函数的运行 但调用此方法的函数会执行完毕
	BindRouter([]RouterHandler) // 绑定这次请求由哪个路由处理
	RouterNext()                // 执行下一个函数
}

type Request struct {
	conn     IConnection     // 已经和客户端建立好的链接
	msg      IMessage        // 客户端请求的数据
	stepLock *sync.RWMutex   // 并发互斥
	needNext bool            // 是否需要执行下一个路由函数
	icResp   IcResp          // 拦截器返回数据
	handlers []RouterHandler // 路由函数切片
	index    int8            // 路由函数切片索引
}

func NewRequest(conn IConnection, msg IMessage) IRequest {
	req := new(Request)
	req.conn = conn
	req.msg = msg
	req.stepLock = new(sync.RWMutex)
	req.needNext = true
	req.index = -1

	return req
}

func (r *Request) GetResponse() IcResp {
	return r.icResp
}

func (r *Request) SetResponse(response IcResp) {
	r.icResp = response
}

func (r *Request) GetMessage() IMessage {
	return r.msg
}

func (r *Request) GetConnection() IConnection {
	return r.conn
}

func (r *Request) GetData() []byte {
	return r.msg.GetData()
}

func (r *Request) GetMsgID() uint32 {
	return r.msg.GetMsgID()
}

func (r *Request) Abort() {
	r.index = int8(len(r.handlers))
}

func (r *Request) BindRouter(handlers []RouterHandler) {
	r.handlers = handlers
}

func (r *Request) RouterNext() {
	r.index++
	for r.index < int8(len(r.handlers)) {
		r.handlers[r.index](r)
		r.index++
	}
}
