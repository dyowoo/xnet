/**
* @File: option.go
* @Author: Jason Woo
* @Date: 2023/8/17 16:00
**/

package xnet

// Option Server的服务Option
type Option func(s *Server)

// WithPacket 只要实现Packet 接口可自由实现数据包解析格式，如果没有则使用默认解析格式
func WithPacket(pack IDataPack) Option {
	return func(s *Server) {
		s.SetPacket(pack)
	}
}

// ClientOption Options for Client
type ClientOption func(c IClient)

func WithPacketClient(pack IDataPack) ClientOption {
	return func(c IClient) {
		c.SetPacket(pack)
	}
}

func WithNameClient(name string) ClientOption {
	return func(c IClient) {
		c.SetName(name)
	}
}
