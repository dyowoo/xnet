/**
* @File: client.go
* @Author: Jason Woo
* @Date: 2023/8/17 15:57
**/

package xnet

import (
	"crypto/tls"
	"fmt"
	"github.com/dyowoo/xnet/xlog"
	"github.com/gorilla/websocket"
	"net"
	"time"
)

type IClient interface {
	Restart()
	Start()
	Stop()
	AddRouter(uint32, ...RouterHandler)
	Conn() IConnection

	// SetOnConnStart 设置该Client的连接创建时Hook函数
	SetOnConnStart(func(IConnection))

	// SetOnConnStop 设置该Client的连接断开时的Hook函数
	SetOnConnStop(func(IConnection))

	// GetOnConnStart 获取该Client的连接创建时Hook函数
	GetOnConnStart() func(IConnection)

	// GetOnConnStop 设置该Client的连接断开时的Hook函数
	GetOnConnStop() func(IConnection)

	// GetPacket 获取Client绑定的数据协议封包方式
	GetPacket() IDataPack

	// SetPacket 设置Client绑定的数据协议封包方式
	SetPacket(IDataPack)

	// GetMsgHandler 获取Client绑定的消息处理模块
	GetMsgHandler() IMsgHandle

	// StartHeartbeat Start 启动心跳检测
	StartHeartbeat(time.Duration)

	// StartHeartBeatWithOption 自定义回调
	StartHeartBeatWithOption(time.Duration, *HeartbeatOption)

	// GetLengthField Get the length field of this Client
	GetLengthField() *LengthField

	// SetDecoder 设置解码器
	SetDecoder(IDecoder)

	// AddInterceptor 添加拦截器
	AddInterceptor(IInterceptor)

	// GetErrChan 获取客户端错误管道
	GetErrChan() chan error

	// SetName 设置客户端Client名称
	SetName(string)

	// GetName 获取客户端Client名称
	GetName() string
}

type Client struct {
	name             string                 // 客户端的名称
	ip               string                 // 目标链接服务器的IP
	port             int                    // 目标链接服务器的端口
	version          string                 // tcp,websocket,客户端版本 tcp,websocket
	conn             IConnection            // 链接实例
	onConnStart      func(conn IConnection) // 该client的连接创建时Hook函数
	onConnStop       func(conn IConnection) // 该client的连接断开时的Hook函数
	packet           IDataPack              // 数据报文封包方式
	exitChan         chan struct{}          // 异步捕获链接关闭状态
	msgHandler       IMsgHandle             // 消息管理模块
	decoder          IDecoder               // 断粘包解码器
	heartbeatChecker IHeartbeatChecker      // 心跳检测器
	useTLS           bool                   // 使用TLS
	dialer           *websocket.Dialer
	errChan          chan error
}

func NewClient(ip string, port int, opts ...ClientOption) IClient {
	c := &Client{
		// 默认名称，可以使用WithNameClient的Option修改
		name:       "XClientTcp",
		ip:         ip,
		port:       port,
		msgHandler: newMsgHandle(),
		packet:     Factory().NewPack(PackTLV),
		decoder:    NewTLVDecoder(),
		version:    "tcp",
		errChan:    make(chan error),
	}

	//  应用Option设置
	for _, opt := range opts {
		opt(c)
	}

	return c
}

func NewWsClient(ip string, port int, opts ...ClientOption) IClient {
	c := &Client{
		// 默认名称，可以使用WithNameClient的Option修改
		name: "XClientWs",
		ip:   ip,
		port: port,

		msgHandler: newMsgHandle(),
		packet:     Factory().NewPack(PackTLV),
		decoder:    NewTLVDecoder(),
		version:    "websocket",
		dialer:     &websocket.Dialer{},
		errChan:    make(chan error),
	}

	// 应用Option设置
	for _, opt := range opts {
		opt(c)
	}

	return c
}

func NewTLSClient(ip string, port int, opts ...ClientOption) IClient {
	c, _ := NewClient(ip, port, opts...).(*Client)

	c.useTLS = true

	return c
}

// Restart 重新启动客户端，发送请求且建立连接
func (c *Client) Restart() {
	c.exitChan = make(chan struct{})

	// 客户端将协程池关闭
	GlobalConfig.WorkerPoolSize = 0

	go func() {
		addr := &net.TCPAddr{
			IP:   net.ParseIP(c.ip),
			Port: c.port,
			Zone: "", //for ipv6, ignore
		}

		// 创建原始Socket，得到net.Conn
		switch c.version {
		case "websocket":
			wsAddr := fmt.Sprintf("ws://%s:%d", c.ip, c.port)

			// 创建原始Socket，得到net.Conn
			wsConn, _, err := c.dialer.Dial(wsAddr, nil)
			if err != nil {
				xlog.ErrorF("wsClient connect to server failed, err:%v", err)
				c.errChan <- err
				return
			}

			c.conn = newWsClientConn(c, wsConn)
		default:
			var conn net.Conn
			var err error
			if c.useTLS {
				config := &tls.Config{
					// 这里是跳过证书验证，因为证书签发机构的CA证书是不被认证的
					InsecureSkipVerify: true,
				}

				conn, err = tls.Dial("tcp", fmt.Sprintf("%v:%v", net.ParseIP(c.ip), c.port), config)
				if err != nil {
					xlog.ErrorF("tls client connect to server failed, err:%v", err)
					c.errChan <- err
					return
				}
			} else {
				conn, err = net.DialTCP("tcp", nil, addr)
				if err != nil {
					xlog.ErrorF("client connect to server failed, err:%v", err)
					c.errChan <- err
					return
				}
			}

			c.conn = newClientConn(c, conn)
		}

		xlog.InfoF("[start] Client LocalAddr: %s, RemoteAddr: %s\n", c.conn.LocalAddr(), c.conn.RemoteAddr())

		if c.heartbeatChecker != nil {
			// 创建链接成功，绑定链接与心跳检测器
			c.heartbeatChecker.BindConn(c.conn)
		}

		go c.conn.Start()

		select {
		case <-c.exitChan:
			xlog.InfoF("client exit.")
		}
	}()
}

// Start 启动客户端，发送请求且建立链接
func (c *Client) Start() {
	// 将解码器添加到拦截器
	if c.decoder != nil {
		c.msgHandler.AddInterceptor(c.decoder)
	}

	c.Restart()
}

// StartHeartbeat 启动心跳检测, interval: 每次发送心跳的时间间隔
func (c *Client) StartHeartbeat(interval time.Duration) {
	checker := NewHeartbeatChecker(interval)

	// 添加心跳检测的路由
	c.AddRouter(checker.MsgID(), checker.RouterHandlers()...)

	// client绑定心跳检测器
	c.heartbeatChecker = checker
}

// StartHeartBeatWithOption 启动心跳检测(自定义回调)
func (c *Client) StartHeartBeatWithOption(interval time.Duration, option *HeartbeatOption) {
	checker := NewHeartbeatChecker(interval)

	if option != nil {
		checker.SetHeartbeatMsgFunc(option.MakeMsg)
		checker.SetOnRemoteNotAlive(option.OnRemoteNotAlive)
		checker.BindRouter(option.HeartbeatMsgID, option.RouterHandlers...)
	}

	c.AddRouter(checker.MsgID(), checker.RouterHandlers()...)

	c.heartbeatChecker = checker
}

func (c *Client) Stop() {
	xlog.InfoF("[stop] client localAddr: %s, remoteAddr: %s\n", c.conn.LocalAddr(), c.conn.RemoteAddr())
	c.conn.Stop()
	c.exitChan <- struct{}{}
	close(c.exitChan)
	close(c.errChan)
}

func (c *Client) AddRouter(msgID uint32, handlers ...RouterHandler) {
	c.msgHandler.AddRouter(msgID, handlers...)
}

func (c *Client) Conn() IConnection {
	return c.conn
}

func (c *Client) SetOnConnStart(hookFunc func(IConnection)) {
	c.onConnStart = hookFunc
}

func (c *Client) SetOnConnStop(hookFunc func(IConnection)) {
	c.onConnStop = hookFunc
}

func (c *Client) GetOnConnStart() func(IConnection) {
	return c.onConnStart
}

func (c *Client) GetOnConnStop() func(IConnection) {
	return c.onConnStop
}

func (c *Client) GetPacket() IDataPack {
	return c.packet
}

func (c *Client) SetPacket(packet IDataPack) {
	c.packet = packet
}

func (c *Client) GetMsgHandler() IMsgHandle {
	return c.msgHandler
}

func (c *Client) AddInterceptor(interceptor IInterceptor) {
	c.msgHandler.AddInterceptor(interceptor)
}

func (c *Client) SetDecoder(decoder IDecoder) {
	c.decoder = decoder
}
func (c *Client) GetLengthField() *LengthField {
	if c.decoder != nil {
		return c.decoder.GetLengthField()
	}
	return nil
}

func (c *Client) GetErrChan() chan error {
	return c.errChan
}

func (c *Client) SetName(name string) {
	c.name = name
}

func (c *Client) GetName() string {
	return c.name
}
