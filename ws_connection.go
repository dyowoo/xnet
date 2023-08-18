/**
* @File: ws_connection.go
* @Author: Jason Woo
* @Date: 2023/8/17 16:45
**/

package xnet

import (
	"context"
	"encoding/hex"
	"errors"
	"github.com/dyowoo/xnet/xlog"
	"github.com/gorilla/websocket"
	"net"
	"sync"
	"time"
)

// WsConnection Websocket连接模块, 用于处理 Websocket 连接的读写业务 一个连接对应一个Connection
type WsConnection struct {
	conn             *websocket.Conn        // 当前连接的socket TCP套接字
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

// newServerConn :for Server, 创建一个Server服务端特性的连接的方法
// Note: 名字由 NewConnection 更变
func newWebsocketConn(server IServer, conn *websocket.Conn, connID uint64) IConnection {
	c := &WsConnection{
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

// newClientConn :for Client, 创建一个Client服务端特性的连接的方法
func newWsClientConn(client IClient, conn *websocket.Conn) IConnection {
	c := &WsConnection{
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

	// 从client继承过来的属性
	c.packet = client.GetPacket()
	c.onConnStart = client.GetOnConnStart()
	c.onConnStop = client.GetOnConnStop()
	c.msgHandler = client.GetMsgHandler()

	return c
}

// StartWriter 写消息Goroutine， 用户将数据发送给客户端
func (c *WsConnection) StartWriter() {
	xlog.InfoF("writer goroutine is running")
	defer xlog.InfoF("%s [conn writer exit!]", c.RemoteAddr().String())

	for {
		select {
		case data, ok := <-c.msgBuffChan:
			if ok {
				if err := c.conn.WriteMessage(websocket.BinaryMessage, data); err != nil {
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

// StartReader 读消息Goroutine，用于从客户端中读取数据
func (c *WsConnection) StartReader() {
	xlog.InfoF("[reader goroutine is running]")
	defer xlog.InfoF("%s [conn reader exit!]", c.RemoteAddr().String())
	defer c.Stop()

	// 创建拆包解包的
	for {
		select {
		case <-c.ctx.Done():
			return
		default:
			// 从conn的IO中读取数据到内存缓冲buffer中
			messageType, buffer, err := c.conn.ReadMessage()
			if err != nil {
				c.cancel()
				return
			}

			if messageType == websocket.PingMessage {
				c.updateActivity()
				continue
			}

			n := len(buffer)
			if err != nil {
				xlog.ErrorF("read msg head [read dataLen=%d], error = %s", n, err.Error())
				return
			}

			xlog.DebugF("read buffer %s \n", hex.EncodeToString(buffer[0:n]))

			// 正常读取到对端数据，更新心跳检测Active状态
			if n > 0 && c.heartbeatChecker != nil {
				c.updateActivity()
			}

			// 处理自定义协议断粘包问题
			if c.frameDecoder != nil {
				// 为读取到的0-n个字节的数据进行解码
				bufArrays := c.frameDecoder.Decode(buffer)
				if bufArrays == nil {
					continue
				}

				for _, bytes := range bufArrays {
					xlog.DebugF("read buffer %s \n", hex.EncodeToString(bytes))
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
func (c *WsConnection) Start() {
	c.ctx, c.cancel = context.WithCancel(context.Background())
	// 按照用户传递进来的创建连接时需要处理的业务，执行钩子方法
	c.callOnConnStart()

	// 启动心跳检测
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
func (c *WsConnection) Stop() {
	c.cancel()
}

func (c *WsConnection) GetConnection() net.Conn {
	return nil
}

func (c *WsConnection) GetWsConn() *websocket.Conn {
	return c.conn
}

func (c *WsConnection) GetConnID() uint64 {
	return c.connID
}

func (c *WsConnection) GetWorkerID() uint32 {
	return c.workerID
}

func (c *WsConnection) RemoteAddr() net.Addr {
	return c.conn.RemoteAddr()
}

func (c *WsConnection) LocalAddr() net.Addr {
	return c.conn.LocalAddr()
}

func (c *WsConnection) Send(data []byte) error {
	c.msgLock.RLock()
	defer c.msgLock.RUnlock()
	if c.isClosed == true {
		return errors.New("wsConnection closed when send msg")
	}

	err := c.conn.WriteMessage(websocket.BinaryMessage, data)
	if err != nil {
		xlog.ErrorF("sendMsg err data = %+v, err = %+v", data, err)
		return err
	}

	return nil
}

func (c *WsConnection) SendToQueue(data []byte) error {
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
		return errors.New("wsConnection closed when send buff msg")
	}

	if data == nil {
		xlog.ErrorF("pack data is nil")
		return errors.New("pack data is nil ")
	}

	select {
	case <-idleTimeout.C:
		return errors.New("send buff msg timeout")
	case c.msgBuffChan <- data:
		return nil
	}
}

// SendMsg 直接将Message数据发送数据给远程的TCP客户端
func (c *WsConnection) SendMsg(msgID uint32, data []byte) error {
	c.msgLock.RLock()
	defer c.msgLock.RUnlock()
	if c.isClosed == true {
		return errors.New("wsConnection closed when send msg")
	}

	// 将data封包，并且发送
	msg, err := c.packet.Pack(NewMsgPackage(msgID, data))
	if err != nil {
		xlog.ErrorF("pack error msg ID = %d", msgID)
		return errors.New("pack error msg ")
	}

	err = c.conn.WriteMessage(websocket.BinaryMessage, msg)
	if err != nil {
		xlog.ErrorF("sendMsg err msg ID = %d, data = %+v, err = %+v", msgID, string(msg), err)
		return err
	}

	return nil
}

// SendBuffMsg sends BuffMsg
func (c *WsConnection) SendBuffMsg(msgID uint32, data []byte) error {
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
		return errors.New("wsConnection closed when send buff msg")
	}

	// 将data封包，并且发送
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

func (c *WsConnection) SetProperty(key string, value any) {
	c.propertyLock.Lock()
	defer c.propertyLock.Unlock()
	if c.property == nil {
		c.property = make(map[string]any)
	}

	c.property[key] = value
}

func (c *WsConnection) GetProperty(key string) (any, error) {
	c.propertyLock.Lock()
	defer c.propertyLock.Unlock()

	if value, ok := c.property[key]; ok {
		return value, nil
	}

	return nil, errors.New("no property found")
}

func (c *WsConnection) RemoveProperty(key string) {
	c.propertyLock.Lock()
	defer c.propertyLock.Unlock()

	delete(c.property, key)
}

// Context 返回ctx，用于用户自定义的go程获取连接退出状态
func (c *WsConnection) Context() context.Context {
	return c.ctx
}

func (c *WsConnection) finalizer() {
	// 如果用户注册了该链接的	关闭回调业务，那么在此刻应该显示调用
	c.callOnConnStop()

	c.msgLock.Lock()
	defer c.msgLock.Unlock()

	// 如果当前链接已经关闭
	if c.isClosed == true {
		return
	}

	// 关闭链接绑定的心跳检测器
	if c.heartbeatChecker != nil {
		c.heartbeatChecker.Stop()
	}

	// 关闭socket链接
	_ = c.conn.Close()

	// 将链接从连接管理器中删除
	if c.connManager != nil {
		c.connManager.Remove(c)
	}

	// 关闭该链接全部管道
	if c.msgBuffChan != nil {
		close(c.msgBuffChan)
	}

	// 设置标志位
	c.isClosed = true

	xlog.InfoF("conn stop()...connID = %d", c.connID)
}

func (c *WsConnection) callOnConnStart() {
	if c.onConnStart != nil {
		xlog.InfoF("callOnConnStart....")
		c.onConnStart(c)
	}
}

func (c *WsConnection) callOnConnStop() {
	if c.onConnStop != nil {
		xlog.InfoF("callOnConnStop....")
		c.onConnStop(c)
	}
}

func (c *WsConnection) IsAlive() bool {
	if c.isClosed {
		return false
	}
	// 检查连接最后一次活动时间，如果超过心跳间隔，则认为连接已经死亡
	return time.Now().Sub(c.lastActivityTime) < GlobalConfig.HeartbeatMaxDuration()
}

func (c *WsConnection) updateActivity() {
	c.lastActivityTime = time.Now()
}

func (c *WsConnection) SetHeartbeat(checker IHeartbeatChecker) {
	c.heartbeatChecker = checker
}

func (c *WsConnection) LocalAddrString() string {
	return c.localAddr
}

func (c *WsConnection) RemoteAddrString() string {
	return c.remoteAddr
}

func (c *WsConnection) GetName() string {
	return c.name
}

func (c *WsConnection) GetMsgHandler() IMsgHandle {
	return c.msgHandler
}
