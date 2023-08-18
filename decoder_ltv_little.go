/**
* @File: decoder_ltv_little.go
* @Author: Jason Woo
* @Date: 2023/8/17 16:19
**/

package xnet

import (
	"bytes"
	"encoding/binary"
	"math"
)

const LtvHeaderSize = 8 //表示TLV空包长度

type LtvLittleDecoder struct {
	Length uint32 //L
	Tag    uint32 //T
	Value  []byte //V
}

func NewLTVLittleDecoder() IDecoder {
	return &LtvLittleDecoder{}
}

func (ltv *LtvLittleDecoder) GetLengthField() *LengthField {
	// +---------------+---------------+---------------+
	// |    Length     |     Tag       |     Value     |
	// | uint32(4byte) | uint32(4byte) |     n byte    |
	// +---------------+---------------+---------------+
	// Length：uint32类型，占4字节，Length标记Value长度
	// Tag：   uint32类型，占4字节
	// Value： 占n字节
	//
	// 说明:
	//    lengthFieldOffset   = 0            (Length的字节位索引下标是4) 长度字段的偏差
	//    lengthFieldLength   = 4            (Length是4个byte) 长度字段占的字节数
	//    lengthAdjustment    = 4            (Length只表示Value长度，程序只会读取Length个字节就结束，后面没有来，故为0，若Value后面还有crc占2字节的话，那么此处就是2。若Length标记的是Tag+Length+Value总长度，那么此处是-8)
	//    initialBytesToStrip = 0            (这个0表示返回完整的协议内容Tag+Length+Value，如果只想返回Value内容，去掉Tag的4字节和Length的4字节，此处就是8) 从解码帧中第一次去除的字节数
	//    maxFrameLength      = 2^32 + 4 + 4 (Length为uint32类型，故2^32次方表示Value最大长度，此外Tag和Length各占4字节)
	// 默认使用TLV封包方式
	return &LengthField{
		MaxFrameLength:      math.MaxUint32 + 4 + 4,
		LengthFieldOffset:   0,
		LengthFieldLength:   4,
		LengthAdjustment:    4,
		InitialBytesToStrip: 0,
		// 注意现在默认是大端，使用小端需要指定编码方式
		Order: binary.LittleEndian,
	}
}

func (ltv *LtvLittleDecoder) decode(data []byte) *LtvLittleDecoder {
	ltvData := LtvLittleDecoder{}

	ltvData.Length = binary.LittleEndian.Uint32(data[0:4])
	ltvData.Tag = binary.LittleEndian.Uint32(data[4:8])
	ltvData.Value = make([]byte, ltvData.Length)

	_ = binary.Read(bytes.NewBuffer(data[8:8+ltvData.Length]), binary.LittleEndian, ltvData.Value)

	return &ltvData
}

func (ltv *LtvLittleDecoder) Intercept(chain IChain) IcResp {
	message := chain.GetIMessage()
	if message == nil {
		return chain.ProceedWithIMessage(message, nil)
	}

	data := message.GetData()

	// 读取的数据不超过包头，直接进入下一层
	if len(data) < LtvHeaderSize {
		return chain.ProceedWithIMessage(message, nil)
	}

	ltvData := ltv.decode(data)

	// 将解码后的数据重新设置到IMessage中,Router需要MsgID来寻址
	message.SetDataLen(ltvData.Length)
	message.SetMsgID(ltvData.Tag)
	message.SetData(ltvData.Value)

	// 将解码后的数据进入下一层
	return chain.ProceedWithIMessage(message, *ltvData)
}
