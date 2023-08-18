/**
* @File: pack_factory.go
* @Author: Jason Woo
* @Date: 2023/8/17 16:02
**/

package xnet

import "sync"

var packOnce sync.Once

type PackFactory struct{}

var factoryInstance *PackFactory

// Factory 生成不同封包解包的方式，单例
func Factory() *PackFactory {
	packOnce.Do(func() {
		factoryInstance = new(PackFactory)
	})

	return factoryInstance
}

// NewPack 创建一个具体的拆包解包对象
func (*PackFactory) NewPack(kind NetDataPack) IDataPack {
	var dataPack IDataPack

	switch kind {
	case PackTLV:
		dataPack = NewDataPack()
	case PackLTV:
		dataPack = NewDataPackLtv()
	default:
		dataPack = NewDataPack()
	}

	return dataPack
}

// NewDecoder 创建一个解码对象
func (*PackFactory) NewDecoder(kind NetDecoder) IDecoder {
	var decoder IDecoder

	switch kind {
	case DecoderTLV:
		decoder = NewTLVDecoder()
	case DecoderLTV:
		decoder = NewLTVLittleDecoder()
	default:
		decoder = NewTLVDecoder()
	}

	return decoder
}
