/**
* @File: data_pack_tlv_big_endian.go
* @Author: Jason Woo
* @Date: 2023/8/17 16:18
**/

package xnet

import (
	"bytes"
	"encoding/binary"
	"errors"
)

type DataPack struct{}

// NewDataPack 封包拆包实例初始化方法
func NewDataPack() IDataPack {
	return &DataPack{}
}

// GetHeadLen 获取包头长度方法
func (dp *DataPack) GetHeadLen() uint32 {
	return defaultHeaderLen
}

// Pack 封包方法,压缩数据
func (dp *DataPack) Pack(msg IMessage) ([]byte, error) {
	// 创建一个存放bytes字节的缓冲
	dataBuff := bytes.NewBuffer([]byte{})

	if err := binary.Write(dataBuff, binary.BigEndian, msg.GetMsgID()); err != nil {
		return nil, err
	}

	if err := binary.Write(dataBuff, binary.BigEndian, msg.GetDataLen()); err != nil {
		return nil, err
	}

	if err := binary.Write(dataBuff, binary.BigEndian, msg.GetData()); err != nil {
		return nil, err
	}

	return dataBuff.Bytes(), nil
}

// Unpack 拆包方法,解压数据
func (dp *DataPack) Unpack(binaryData []byte) (IMessage, error) {
	dataBuff := bytes.NewReader(binaryData)

	// 只解压head的信息，得到dataLen和msgID
	msg := &Message{}

	if err := binary.Read(dataBuff, binary.BigEndian, &msg.ID); err != nil {
		return nil, err
	}

	if err := binary.Read(dataBuff, binary.BigEndian, &msg.DataLen); err != nil {
		return nil, err
	}

	// 判断dataLen的长度是否超出我们允许的最大包长度
	if GlobalConfig.MaxPacketSize > 0 && msg.GetDataLen() > GlobalConfig.MaxPacketSize {
		return nil, errors.New("too large msg data received")
	}

	// 这里只需要把head的数据拆包出来就可以了，然后再通过head的长度，再从conn读取一次数据
	return msg, nil
}
