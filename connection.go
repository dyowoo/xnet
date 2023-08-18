/**
* @File: connection.go
* @Author: Jason Woo
* @Date: 2023/8/17 13:18
**/

package xnet

import (
	"context"
	"errors"
	"github.com/dyowoo/xnet/xlog"
	"github.com/gorilla/websocket"
	"net"
	"sync"
	"time"
)

type IConnection interface {
	Start()                                 // Start 启动连接，让当前连接开始工作
	Stop()                                  // Stop 停止连接，结束当前连接状态
	Context() context.Context               // Context 返回ctx，用于用户自定义的go程获取连接退出状态
	GetName() string                        // 获取当前连接名称
	GetConnection() net.Conn                // 从当前连接获取原始的socket
	GetWsConn() *websocket.Conn             // 从当前连接中获取原始的websocket连接
	GetConnID() uint64                      // 获取当前连接ID
	GetMsgHandler() IMsgHandle              // 获取消息处理器
	GetWorkerID() uint32                    // 获取workerId
	RemoteAddr() net.Addr                   // 获取链接远程地址信息
	LocalAddr() net.Addr                    // 获取链接本地地址信息
	RemoteAddrString() string               // 获取链接远程地址信息
	LocalAddrString() string                // 获取链接本地地址信息
	Send([]byte) error                      // Send 直接发送数据
	SendToQueue([]byte) error               // Send 发送到队列
	SendMsg(uint32, []byte) error           // 直接将Message数据发送数据给远程的TCP客户端(无缓冲)
	SendBuffMsg(uint32, []byte) error       // 直接将Message数据发送给远程的TCP客户端(有缓冲)
	SetProperty(string, any)                // Set connection property
	GetProperty(string) (any, error)        // Get connection property
	RemoveProperty(string)                  // Remove connection property
	IsAlive() bool                          // 判断当前连接是否存活
	SetHeartbeat(checker IHeartbeatChecker) // 设置心跳检测器
}

// Connection (用于处理Tcp连接的读写业务 一个连接对应一个Connection)
type Connection struct {
	conn             net.Conn               // 当前连接的socket TCP套接字
	connID           uint64                 // 当前连接的ID
	workerID         uint32                 // 负责处理该链接的workerID
	msgHandler       IMsgHandle             // 消息管理MsgID和对应处理方法的消息管理模块
	ctx              context.Context        // 告知该链接已经退出
	cancel           context.CancelFunc     // 停止的channel
	msgBuffChan      chan []byte            // 有缓冲管道，用于读、写两个goroutine之间的消息通信
	msgLock          sync.RWMutex           // 用户收发消息的Lock
	property         map[string]any         // 链接属性
	propertyLock     sync.Mutex             // 保护当前property的锁
	isClosed         bool                   // 当前连接的关闭状态
	connManager      IConnManager           // 当前链接是属于哪个Connection Manager的
	onConnStart      func(conn IConnection) // 当前连接创建时Hook函数
	onConnStop       func(conn IConnection) // 当前连接断开时的Hook函数
	packet           IDataPack              // 数据报文封包方式
	lastActivityTime time.Time              // 最后一次活动时间
	frameDecoder     IFrameDecoder          // 断粘包解码器
	heartbeatChecker IHeartbeatChecker      // 心跳检测器
	name             string                 // 链接名称，默认与创建链接的Server/Client的Name一致
	localAddr        string                 // 当前链接的本地地址
	remoteAddr       string                 // 当前链接的远程地址
}

// 创建一个Server服务端特性的连接的方法
func newServerConn(server IServer, conn net.Conn, connID uint64) IConnection {
	c := &Connection{
		conn:        conn,
		connID:      connID,
		isClosed:    false,
		msgBuffChan: nil,
		property:    nil,
		name:        server.ServerName(),
		localAddr:   conn.LocalAddr().String(),
		remoteAddr:  conn.RemoteAddr().String(),
	}

	lengthField := server.GetLengthField()
	if lengthField != nil {
		c.frameDecoder = NewFrameDecoder(*lengthField)
	}

	// 从server继承过来的属性
	c.packet = server.GetPacket()
	c.onConnStart = server.GetOnConnStart()
	c.onConnStop = server.GetOnConnStop()
	c.msgHandler = server.GetMsgHandler()

	// 将当前的Connection与Server的ConnManager绑定
	c.connManager = server.GetConnMgr()

	// 将新创建的Conn添加到链接管理中
	server.GetConnMgr().Add(c)

	return c
}

// 创建一个Client服务端特性的连接的方法
func newClientConn(client IClient, conn net.Conn) IConnection {
	c := &Connection{
		conn:        conn,
		connID:      0,
		isClosed:    false,
		msgBuffChan: nil,
		property:    nil,
		name:        client.GetName(),
		localAddr:   conn.LocalAddr().String(),
		remoteAddr:  conn.RemoteAddr().String(),
	}

	lengthField := client.GetLengthField()
	if lengthField != nil {
		c.frameDecoder = NewFrameDecoder(*lengthField)
	}

	//  从client继承过来的属性
	c.packet = client.GetPacket()
	c.onConnStart = client.GetOnConnStart()
	c.onConnStop = client.GetOnConnStop()
	c.msgHandler = client.GetMsgHandler()

	return c
}

// StartWriter 写消息Goroutine， 用户将数据发送给客户端
func (c *Connection) StartWriter() {
	xlog.InfoF("writer goroutine is running")
	defer xlog.InfoF("%s [conn writer exit!]", c.RemoteAddr().String())

	for {
		select {
		case data, ok := <-c.msgBuffChan:
			if ok {
				if _, err := c.conn.Write(data); err != nil {
					xlog.ErrorF("send buff data error:, %s conn writer exit", err)
					break
				}

			} else {
				xlog.ErrorF("msgBuffChan is closed")
				break
			}
		case <-c.ctx.Done():
			return
		}
	}
}

// StartReader (读消息Goroutine，用于从客户端中读取数据)
func (c *Connection) StartReader() {
	xlog.InfoF("[reader goroutine is running]")
	defer xlog.InfoF("%s [conn reader exit!]", c.RemoteAddr().String())
	defer c.Stop()
	defer func() {
		if err := recover(); err != nil {
			xlog.ErrorF("connID=%d, panic err=%v", c.GetConnID(), err)
		}
	}()

	for {
		select {
		case <-c.ctx.Done():
			return
		default:
			buffer := make([]byte, GlobalConfig.IOReadBuffSize)

			// 从conn的IO中读取数据到内存缓冲buffer中
			n, err := c.conn.Read(buffer)
			if err != nil {
				xlog.ErrorF("read msg head [read dataLen=%d], error = %s", n, err)
				return
			}

			// 正常读取到对端数据，更新心跳检测Active状态
			if n > 0 && c.heartbeatChecker != nil {
				c.updateActivity()
			}

			// 处理自定义协议断粘包问题
			if c.frameDecoder != nil {
				// 为读取到的0-n个字节的数据进行解码
				bufArrays := c.frameDecoder.Decode(buffer[0:n])
				if bufArrays == nil {
					continue
				}
				for _, bytes := range bufArrays {
					msg := NewMessage(uint32(len(bytes)), bytes)
					// 得到当前客户端请求的Request数据
					req := NewRequest(c, msg)
					c.msgHandler.Execute(req)
				}
			} else {
				msg := NewMessage(uint32(n), buffer[0:n])
				// 得到当前客户端请求的Request数据
				req := NewRequest(c, msg)
				c.msgHandler.Execute(req)
			}
		}
	}
}

// Start 启动连接，让当前连接开始工作
func (c *Connection) Start() {
	defer func() {
		if err := recover(); err != nil {
			xlog.ErrorF("Connection Start() error: %v", err)
		}
	}()
	c.ctx, c.cancel = context.WithCancel(context.Background())

	// 按照用户传递进来的创建连接时需要处理的业务，执行钩子方法
	c.callOnConnStart()

	if c.heartbeatChecker != nil {
		c.heartbeatChecker.Start()
		c.updateActivity()
	}

	c.workerID = useWorker(c)

	// 开启用户从客户端读取数据流程的Goroutine
	go c.StartReader()

	select {
	case <-c.ctx.Done():
		c.finalizer()

		freeWorker(c)
		return
	}
}

// Stop 停止连接，结束当前连接状态
func (c *Connection) Stop() {
	c.cancel()
}

func (c *Connection) GetConnection() net.Conn {
	return c.conn
}

func (c *Connection) GetWsConn() *websocket.Conn {
	return nil
}

func (c *Connection) GetConnID() uint64 {
	return c.connID
}

func (c *Connection) GetWorkerID() uint32 {
	return c.workerID
}

func (c *Connection) RemoteAddr() net.Addr {
	return c.conn.RemoteAddr()
}

func (c *Connection) LocalAddr() net.Addr {
	return c.conn.LocalAddr()
}

func (c *Connection) Send(data []byte) error {
	c.msgLock.RLock()
	defer c.msgLock.RUnlock()
	if c.isClosed == true {
		return errors.New("connection closed when send msg")
	}

	_, err := c.conn.Write(data)
	if err != nil {
		xlog.ErrorF("sendMsg err data = %+v, err = %+v", data, err)
		return err
	}

	return nil
}

func (c *Connection) SendToQueue(data []byte) error {
	c.msgLock.RLock()
	defer c.msgLock.RUnlock()

	if c.msgBuffChan == nil {
		c.msgBuffChan = make(chan []byte, GlobalConfig.MaxMsgChanLen)
		// 开启用于写回客户端数据流程的Goroutine
		// 此方法只读取MsgBuffChan中的数据没调用SendBuffMsg可以分配内存和启用协程
		go c.StartWriter()
	}

	idleTimeout := time.NewTimer(5 * time.Millisecond)
	defer idleTimeout.Stop()

	if c.isClosed == true {
		return errors.New("connection closed when send buff msg")
	}

	if data == nil {
		xlog.ErrorF("pack data is nil")
		return errors.New("pack data is nil")
	}

	select {
	case <-idleTimeout.C:
		return errors.New("send buff msg timeout")
	case c.msgBuffChan <- data:
		return nil
	}
}

// SendMsg 直接将Message数据发送数据给远程的TCP客户端
func (c *Connection) SendMsg(msgID uint32, data []byte) error {
	c.msgLock.RLock()
	defer c.msgLock.RUnlock()
	if c.isClosed == true {
		return errors.New("connection closed when send msg")
	}

	// Pack data and send it
	msg, err := c.packet.Pack(NewMsgPackage(msgID, data))
	if err != nil {
		xlog.ErrorF("pack error msg ID = %d", msgID)
		return errors.New("pack error msg ")
	}

	_, err = c.conn.Write(msg)
	if err != nil {
		xlog.ErrorF("sendMsg err msg ID = %d, data = %+v, err = %+v", msgID, string(msg), err)
		return err
	}

	return nil
}

func (c *Connection) SendBuffMsg(msgID uint32, data []byte) error {
	c.msgLock.RLock()
	defer c.msgLock.RUnlock()

	if c.msgBuffChan == nil {
		c.msgBuffChan = make(chan []byte, GlobalConfig.MaxMsgChanLen)
		// 开启用于写回客户端数据流程的Goroutine
		// 此方法只读取MsgBuffChan中的数据没调用SendBuffMsg可以分配内存和启用协程
		go c.StartWriter()
	}

	idleTimeout := time.NewTimer(5 * time.Millisecond)
	defer idleTimeout.Stop()

	if c.isClosed == true {
		return errors.New("connection closed when send buff msg")
	}

	msg, err := c.packet.Pack(NewMsgPackage(msgID, data))
	if err != nil {
		xlog.ErrorF("pack error msg ID = %d", msgID)
		return errors.New("pack error msg ")
	}

	select {
	case <-idleTimeout.C:
		return errors.New("send buff msg timeout")
	case c.msgBuffChan <- msg:
		return nil
	}
}

func (c *Connection) SetProperty(key string, value any) {
	c.propertyLock.Lock()
	defer c.propertyLock.Unlock()
	if c.property == nil {
		c.property = make(map[string]any)
	}

	c.property[key] = value
}

func (c *Connection) GetProperty(key string) (any, error) {
	c.propertyLock.Lock()
	defer c.propertyLock.Unlock()

	if value, ok := c.property[key]; ok {
		return value, nil
	}

	return nil, errors.New("no property found")
}

func (c *Connection) RemoveProperty(key string) {
	c.propertyLock.Lock()
	defer c.propertyLock.Unlock()

	delete(c.property, key)
}

func (c *Connection) Context() context.Context {
	return c.ctx
}

func (c *Connection) finalizer() {
	// 如果用户注册了该链接的	关闭回调业务，那么在此刻应该显示调用
	c.callOnConnStop()

	c.msgLock.Lock()
	defer c.msgLock.Unlock()

	if c.isClosed == true {
		return
	}

	if c.heartbeatChecker != nil {
		c.heartbeatChecker.Stop()
	}

	_ = c.conn.Close()

	if c.connManager != nil {
		c.connManager.Remove(c)
	}

	if c.msgBuffChan != nil {
		close(c.msgBuffChan)
	}

	c.isClosed = true

	xlog.InfoF("conn stop()...connID = %d", c.connID)
}

func (c *Connection) callOnConnStart() {
	if c.onConnStart != nil {
		xlog.InfoF("callOnConnStart....")
		c.onConnStart(c)
	}
}

func (c *Connection) callOnConnStop() {
	if c.onConnStop != nil {
		xlog.InfoF("callOnConnStop....")
		c.onConnStop(c)
	}
}

func (c *Connection) IsAlive() bool {
	if c.isClosed {
		return false
	}
	// 检查连接最后一次活动时间，如果超过心跳间隔，则认为连接已经死亡
	return time.Now().Sub(c.lastActivityTime) < GlobalConfig.HeartbeatMaxDuration()
}

func (c *Connection) updateActivity() {
	c.lastActivityTime = time.Now()
}

func (c *Connection) SetHeartbeat(checker IHeartbeatChecker) {
	c.heartbeatChecker = checker
}

func (c *Connection) LocalAddrString() string {
	return c.localAddr
}

func (c *Connection) RemoteAddrString() string {
	return c.remoteAddr
}

func (c *Connection) GetName() string {
	return c.name
}

func (c *Connection) GetMsgHandler() IMsgHandle {
	return c.msgHandler
}
