/**
* @File: data_pack.go
* @Author: Jason Woo
* @Date: 2023/8/17 15:41
**/

package xnet

var defaultHeaderLen uint32 = 8

type IDataPack interface {
	GetHeadLen() uint32              // 获取包头长度方法
	Pack(IMessage) ([]byte, error)   // 封包方法
	Unpack([]byte) (IMessage, error) // 拆包方法
}

type NetDataPack int

const (
	PackTLV NetDataPack = iota
	PackLTV
)

type NetDecoder int

const (
	DecoderTLV NetDecoder = iota
	DecoderLTV
)
