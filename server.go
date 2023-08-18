/**
* @File: server.go
* @Author: Jason Woo
* @Date: 2023/8/17 15:57
**/

package xnet

import (
	"crypto/rand"
	"crypto/tls"
	"errors"
	"fmt"
	"github.com/dyowoo/xnet/xlog"
	"github.com/gorilla/websocket"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"
)

type IServer interface {
	Start()                                                   // 启动服务器方法
	Stop()                                                    // 停止服务器方法
	Serve()                                                   // 开启业务服务方法
	AddRouter(uint32, ...RouterHandler) IRouter               // 路由功能
	Use(...RouterHandler) IRouter                             // 公共组件管理
	UseGateway(RouterHandler)                                 // 使用网关
	GetConnMgr() IConnManager                                 // 得到链接管理
	SetOnConnStart(func(IConnection))                         // 设置该Server的连接创建时Hook函数
	SetOnConnStop(func(IConnection))                          // 设置该Server的连接断开时的Hook函数
	GetOnConnStart() func(IConnection)                        // 得到该Server的连接创建时Hook函数
	GetOnConnStop() func(IConnection)                         // 得到该Server的连接断开时的Hook函数
	GetPacket() IDataPack                                     // 获取Server绑定的数据协议封包方式
	GetMsgHandler() IMsgHandle                                // 获取Server绑定的消息处理模块
	SetPacket(IDataPack)                                      // 设置Server绑定的数据协议封包方式
	StartHeartbeat(time.Duration)                             // 启动心跳检测
	StartHeartbeatWithOption(time.Duration, *HeartbeatOption) // 启动心跳检测(自定义回调)
	GetHeartbeat() IHeartbeatChecker                          // 获取心跳检测器
	GetLengthField() *LengthField                             //
	SetDecoder(IDecoder)                                      //
	AddInterceptor(IInterceptor)                              //
	SetWebsocketAuth(func(r *http.Request) error)             // 添加websocket认证方法
	ServerName() string                                       // 获取服务器名称
}

// Server 接口实现，定义一个Server服务类
type Server struct {
	name             string // 服务器的名称
	ipVersion        string
	ip               string                 // 服务绑定的IP地址
	port             int                    // 服务绑定的端口
	wsPort           int                    // 服务绑定的websocket 端口 (Websocket port the server is bound to)
	msgHandler       IMsgHandle             // 当前Server的消息管理模块，用来绑定MsgID和对应的处理方法
	connMgr          IConnManager           // 当前Server的链接管理器
	onConnStart      func(conn IConnection) // 该Server的连接创建时Hook函数
	onConnStop       func(conn IConnection) // 该Server的连接断开时的Hook函数
	packet           IDataPack              // 数据报文封包方式
	exitChan         chan struct{}          // 异步捕获链接关闭状态
	decoder          IDecoder               // 断粘包解码器
	heartbeatChecker IHeartbeatChecker      // 心跳检测器
	upgrader         *websocket.Upgrader
	websocketAuth    func(r *http.Request) error
	cid              uint64
}

// 根据config创建一个服务器句柄
func newServerWithConfig(config *Config, ipVersion string, opts ...Option) IServer {
	PrintLogo()

	s := &Server{
		name:       config.Name,
		ipVersion:  ipVersion,
		ip:         config.Host,
		port:       config.TCPPort,
		wsPort:     config.WsPort,
		msgHandler: newMsgHandle(),
		connMgr:    newConnManager(),
		exitChan:   nil,
		packet:     Factory().NewPack(PackTLV),
		decoder:    Factory().NewDecoder(DecoderTLV),
		upgrader: &websocket.Upgrader{
			ReadBufferSize: int(config.IOReadBuffSize),
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		},
	}

	for _, opt := range opts {
		opt(s)
	}

	// 提示当前配置信息
	config.Show()

	s.Use(RouterRecovery)

	return s
}

// DefaultServer 创建一个服务器句柄
func DefaultServer(opts ...Option) IServer {
	return newServerWithConfig(GlobalConfig, "tcp", opts...)
}

// NewServer 创建一个服务器句柄
func NewServer(config Config, opts ...Option) IServer {
	// 刷新用户配置到全局配置变量
	UserConfToGlobal(&config)

	s := newServerWithConfig(&config, "tcp4", opts...)
	return s
}

func (s *Server) StartConn(conn IConnection) {
	if s.heartbeatChecker != nil {
		heartBeatChecker := s.heartbeatChecker.Clone()

		heartBeatChecker.BindConn(conn)
	}

	conn.Start()
}

func (s *Server) ListenTcpConn() {
	addr, err := net.ResolveTCPAddr(s.ipVersion, fmt.Sprintf("%s:%d", s.ip, s.port))
	if err != nil {
		xlog.ErrorF("[start] resolve tcp addr err: %v\n", err)
		return
	}

	var listener net.Listener
	if GlobalConfig.CertFile != "" && GlobalConfig.PrivateKeyFile != "" {
		crt, err := tls.LoadX509KeyPair(GlobalConfig.CertFile, GlobalConfig.PrivateKeyFile)
		if err != nil {
			panic(err)
		}

		tlsConfig := &tls.Config{}
		tlsConfig.Certificates = []tls.Certificate{crt}
		tlsConfig.Time = time.Now
		tlsConfig.Rand = rand.Reader
		listener, err = tls.Listen(s.ipVersion, fmt.Sprintf("%s:%d", s.ip, s.port), tlsConfig)
		if err != nil {
			panic(err)
		}
	} else {
		listener, err = net.ListenTCP(s.ipVersion, addr)
		if err != nil {
			panic(err)
		}
	}

	go func() {
		for {
			// 设置服务器最大连接控制,如果超过最大连接，则等待
			if s.connMgr.Len() >= GlobalConfig.MaxConn {
				xlog.InfoF("exceeded the maxConnNum:%d, wait:%d", GlobalConfig.MaxConn, AcceptDelay.duration)
				AcceptDelay.Delay()
				continue
			}
			// 阻塞等待客户端建立连接请求
			conn, err := listener.Accept()
			if err != nil {
				if errors.Is(err, net.ErrClosed) {
					xlog.ErrorF("listener closed")
					return
				}
				xlog.ErrorF("accept err: %v", err)
				AcceptDelay.Delay()
				continue
			}

			AcceptDelay.Reset()

			// 处理该新连接请求的 业务 方法， 此时应该有 handler 和 conn是绑定的
			newCid := atomic.AddUint64(&s.cid, 1)
			dealConn := newServerConn(s, conn, newCid)

			go s.StartConn(dealConn)

		}
	}()

	select {
	case <-s.exitChan:
		err := listener.Close()
		if err != nil {
			xlog.ErrorF("listener close err: %v", err)
		}
	}
}

func (s *Server) ListenWebsocketConn() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// 设置服务器最大连接控制,如果超过最大连接，则等待
		if s.connMgr.Len() >= GlobalConfig.MaxConn {
			xlog.InfoF("exceeded the maxConnNum:%d, wait:%d", GlobalConfig.MaxConn, AcceptDelay.duration)
			AcceptDelay.Delay()
			return
		}

		// 如果需要 websocket 认证请设置认证信息
		if s.websocketAuth != nil {
			err := s.websocketAuth(r)
			if err != nil {
				xlog.ErrorF(" websocket auth err:%v", err)
				w.WriteHeader(401)
				AcceptDelay.Delay()
				return
			}
		}

		// 判断 header 里面是有子协议
		if len(r.Header.Get("Sec-Websocket-Protocol")) > 0 {
			s.upgrader.Subprotocols = websocket.Subprotocols(r)
		}

		// 升级成 websocket 连接
		conn, err := s.upgrader.Upgrade(w, r, nil)
		if err != nil {
			xlog.ErrorF("new websocket err:%v", err)
			w.WriteHeader(500)
			AcceptDelay.Delay()
			return
		}
		AcceptDelay.Reset()

		// 处理该新连接请求的 业务 方法， 此时应该有 handler 和 conn是绑定的
		newCid := atomic.AddUint64(&s.cid, 1)
		wsConn := newWebsocketConn(s, conn, newCid)

		go s.StartConn(wsConn)
	})

	err := http.ListenAndServe(fmt.Sprintf("%s:%d", s.ip, s.wsPort), nil)
	if err != nil {
		panic(err)
	}
}

// Start 开启网络服务
func (s *Server) Start() {
	xlog.InfoF("[start] server name: %s,listener at ip: %s, port %d is starting", s.name, s.ip, s.port)
	s.exitChan = make(chan struct{})

	// 将解码器添加到拦截器
	if s.decoder != nil {
		s.msgHandler.AddInterceptor(s.decoder)
	}

	// 启动worker工作池机制
	s.msgHandler.StartWorkerPool()

	// 开启一个go去做服务端Listener业务
	switch GlobalConfig.Mode {
	case ServerModeTcp:
		go s.ListenTcpConn()
	case ServerModeWebsocket:
		go s.ListenWebsocketConn()
	default:
		go s.ListenTcpConn()
		go s.ListenWebsocketConn()
	}
}

// Stop 停止服务
func (s *Server) Stop() {
	xlog.InfoF("[stop] fastnet2 server, name %s", s.name)

	// 将其他需要清理的连接信息或者其他信息 也要一并停止或者清理
	s.connMgr.ClearConn()
	s.exitChan <- struct{}{}
	close(s.exitChan)
}

// Serve 运行服务
func (s *Server) Serve() {
	s.Start()
	// 阻塞,否则主Go退出
	c := make(chan os.Signal, 1)
	// 监听指定信号 ctrl+c kill信号
	signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
	sig := <-c
	xlog.InfoF("[serve] fastnet2 server, name %s, serve interrupt, signal = %v", s.name, sig)
}

func (s *Server) AddRouter(msgID uint32, router ...RouterHandler) IRouter {
	return s.msgHandler.AddRouter(msgID, router...)
}

func (s *Server) Use(Handlers ...RouterHandler) IRouter {
	return s.msgHandler.Use(Handlers...)
}

func (s *Server) UseGateway(handler RouterHandler) {
	s.msgHandler.UseGateway(handler)
}

func (s *Server) GetConnMgr() IConnManager {
	return s.connMgr
}

func (s *Server) SetOnConnStart(hookFunc func(IConnection)) {
	s.onConnStart = hookFunc
}

func (s *Server) SetOnConnStop(hookFunc func(IConnection)) {
	s.onConnStop = hookFunc
}

func (s *Server) GetOnConnStart() func(IConnection) {
	return s.onConnStart
}

func (s *Server) GetOnConnStop() func(IConnection) {
	return s.onConnStop
}

func (s *Server) GetPacket() IDataPack {
	return s.packet
}

func (s *Server) SetPacket(packet IDataPack) {
	s.packet = packet
}

func (s *Server) GetMsgHandler() IMsgHandle {
	return s.msgHandler
}

// StartHeartbeat 启动心跳检测
// interval 每次发送心跳的时间间隔
func (s *Server) StartHeartbeat(interval time.Duration) {
	checker := NewHeartbeatChecker(interval)

	// 添加心跳检测的路由
	s.AddRouter(checker.MsgID(), checker.RouterHandlers()...)

	// server绑定心跳检测器
	s.heartbeatChecker = checker
}

// StartHeartbeatWithOption 启动心跳检测
// option 心跳检测的配置
func (s *Server) StartHeartbeatWithOption(interval time.Duration, option *HeartbeatOption) {
	checker := NewHeartbeatChecker(interval)

	if option != nil {
		checker.SetHeartbeatMsgFunc(option.MakeMsg)
		checker.SetOnRemoteNotAlive(option.OnRemoteNotAlive)
		checker.BindRouter(option.HeartbeatMsgID, option.RouterHandlers...)
	}

	// 添加心跳检测的路由
	s.AddRouter(checker.MsgID(), checker.RouterHandlers()...)

	// server绑定心跳检测器
	s.heartbeatChecker = checker
}

func (s *Server) GetHeartbeat() IHeartbeatChecker {
	return s.heartbeatChecker
}

func (s *Server) SetDecoder(decoder IDecoder) {
	s.decoder = decoder
}

func (s *Server) GetLengthField() *LengthField {
	if s.decoder != nil {
		return s.decoder.GetLengthField()
	}
	return nil
}

func (s *Server) AddInterceptor(interceptor IInterceptor) {
	s.msgHandler.AddInterceptor(interceptor)
}

func (s *Server) SetWebsocketAuth(f func(r *http.Request) error) {
	s.websocketAuth = f
}

func (s *Server) ServerName() string {
	return s.name
}
