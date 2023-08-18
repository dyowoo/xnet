/**
* @File: frame_decoder.go
* @Author: Jason Woo
* @Date: 2023/8/17 16:32
**/

package xnet

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"sync"
)

type FrameDecoder struct {
	LengthField //从ILengthField集成的基础属性

	LengthFieldEndOffset   int   //长度字段结束位置的偏移量  LengthFieldOffset+LengthFieldLength
	failFast               bool  //快速失败
	discardingTooLongFrame bool  //true 表示开启丢弃模式，false 正常工作模式
	tooLongFrameLength     int64 //当某个数据包的长度超过maxLength，则开启丢弃模式，此字段记录需要丢弃的数据长度
	bytesToDiscard         int64 //记录还剩余多少字节需要丢弃
	in                     []byte
	lock                   sync.Mutex
}

func NewFrameDecoder(lf LengthField) IFrameDecoder {
	frameDecoder := new(FrameDecoder)

	// 基础属性赋值
	if lf.Order == nil {
		frameDecoder.Order = binary.BigEndian
	} else {
		frameDecoder.Order = lf.Order
	}
	frameDecoder.MaxFrameLength = lf.MaxFrameLength
	frameDecoder.LengthFieldOffset = lf.LengthFieldOffset
	frameDecoder.LengthFieldLength = lf.LengthFieldLength
	frameDecoder.LengthAdjustment = lf.LengthAdjustment
	frameDecoder.InitialBytesToStrip = lf.InitialBytesToStrip

	frameDecoder.LengthFieldEndOffset = lf.LengthFieldOffset + lf.LengthFieldLength
	frameDecoder.in = make([]byte, 0)

	return frameDecoder
}

func NewFrameDecoderByParams(maxFrameLength uint64, lengthFieldOffset, lengthFieldLength, lengthAdjustment, initialBytesToStrip int) IFrameDecoder {
	return NewFrameDecoder(LengthField{
		MaxFrameLength:      maxFrameLength,
		LengthFieldOffset:   lengthFieldOffset,
		LengthFieldLength:   lengthFieldLength,
		LengthAdjustment:    lengthAdjustment,
		InitialBytesToStrip: initialBytesToStrip,
		Order:               binary.BigEndian,
	})
}

func (d *FrameDecoder) fail(frameLength int64) {
	//丢弃完成或未完成都抛异常
	//if frameLength > 0 {
	//	msg := fmt.Sprintf("Adjusted frame length exceeds %d : %d - discarded", this.MaxFrameLength, frameLength)
	//	panic(msg)
	//} else {
	//	msg := fmt.Sprintf("Adjusted frame length exceeds %d - discarded", this.MaxFrameLength)
	//	panic(msg)
	//}
}

func (d *FrameDecoder) discardingTooLongFrameFunc(buffer *bytes.Buffer) {
	// 保存还需丢弃多少字节
	bytesToDiscard := d.bytesToDiscard
	// 获取当前可以丢弃的字节数，有可能出现半包
	localBytesToDiscard := math.Min(float64(bytesToDiscard), float64(buffer.Len()))
	// 丢弃
	buffer.Next(int(localBytesToDiscard))
	// 更新还需丢弃的字节数
	bytesToDiscard -= int64(localBytesToDiscard)
	d.bytesToDiscard = bytesToDiscard
	// 是否需要快速失败，回到上面的逻辑
	d.failIfNecessary(false)
}

func (d *FrameDecoder) getUnadjustedFrameLength(buf *bytes.Buffer, offset int, length int, order binary.ByteOrder) int64 {
	// 长度字段的值
	var frameLength int64
	arr := buf.Bytes()
	arr = arr[offset : offset+length]
	buffer := bytes.NewBuffer(arr)
	switch length {
	case 1:
		// byte
		var value uint8
		err := binary.Read(buffer, order, &value)
		if err != nil {
			return 0
		}
		frameLength = int64(value)
	case 2:
		// short
		var value uint16
		err := binary.Read(buffer, order, &value)
		if err != nil {
			return 0
		}
		frameLength = int64(value)
	case 3:
		// int占32位，这里取出后24位，返回int类型
		if order == binary.LittleEndian {
			n := uint(arr[0]) | uint(arr[1])<<8 | uint(arr[2])<<16
			frameLength = int64(n)
		} else {
			n := uint(arr[2]) | uint(arr[1])<<8 | uint(arr[0])<<16
			frameLength = int64(n)
		}
	case 4:
		// int
		var value uint32
		err := binary.Read(buffer, order, &value)
		if err != nil {
			return 0
		}
		frameLength = int64(value)
	case 8:
		// long
		err := binary.Read(buffer, order, &frameLength)
		if err != nil {
			return 0
		}
	default:
		panic(fmt.Sprintf("unsupported LengthFieldLength: %d (expected: 1, 2, 3, 4, or 8)", d.LengthFieldLength))
	}
	return frameLength
}

func (d *FrameDecoder) failOnNegativeLengthField(in *bytes.Buffer, frameLength int64, lengthFieldEndOffset int) {
	in.Next(lengthFieldEndOffset)
	panic(fmt.Sprintf("negative pre-adjustment length field: %d", frameLength))
}

func (d *FrameDecoder) failIfNecessary(firstDetectionOfTooLongFrame bool) {
	if d.bytesToDiscard == 0 {
		// 说明需要丢弃的数据已经丢弃完成
		// 保存一下被丢弃的数据包长度
		tooLongFrameLength := d.tooLongFrameLength
		d.tooLongFrameLength = 0
		// 关闭丢弃模式
		d.discardingTooLongFrame = false
		// failFast：默认true
		// firstDetectionOfTooLongFrame：传入true
		if !d.failFast || firstDetectionOfTooLongFrame {
			// 快速失败
			d.fail(tooLongFrameLength)
		}
	} else {
		// 说明还未丢弃完成
		if d.failFast && firstDetectionOfTooLongFrame {
			// 快速失败
			d.fail(d.tooLongFrameLength)
		}
	}
}

// frameLength：数据包的长度
func (d *FrameDecoder) exceededFrameLength(in *bytes.Buffer, frameLength int64) {
	// 数据包长度-可读的字节数  两种情况
	// 1. 数据包总长度为100，可读的字节数为50，说明还剩余50个字节需要丢弃但还未接收到
	// 2. 数据包总长度为100，可读的字节数为150，说明缓冲区已经包含了整个数据包
	discard := frameLength - int64(in.Len())
	// 记录一下最大的数据包的长度
	d.tooLongFrameLength = frameLength
	if discard < 0 {
		// 说明是第二种情况，直接丢弃当前数据包
		in.Next(int(frameLength))
	} else {
		// 说明是第一种情况，还有部分数据未接收到
		// 开启丢弃模式
		d.discardingTooLongFrame = true
		// 记录下次还需丢弃多少字节
		d.bytesToDiscard = discard
		// 丢弃缓冲区所有数据
		in.Next(in.Len())
	}
	// 跟进去
	d.failIfNecessary(true)
}

func (d *FrameDecoder) failOnFrameLengthLessThanInitialBytesToStrip(in *bytes.Buffer, frameLength int64, initialBytesToStrip int) {
	in.Next(int(frameLength))
	panic(fmt.Sprintf("adjusted frame length (%d) is less  than initialBytesToStrip: %d", frameLength, initialBytesToStrip))
}

func (d *FrameDecoder) decode(buf []byte) []byte {
	in := bytes.NewBuffer(buf)
	// 丢弃模式
	if d.discardingTooLongFrame {
		d.discardingTooLongFrameFunc(in)
	}
	// 判断缓冲区中可读的字节数是否小于长度字段的偏移量
	if in.Len() < d.LengthFieldEndOffset {
		// 说明长度字段的包都还不完整，半包
		return nil
	}
	// 执行到这，说明可以解析出长度字段的值了

	// 计算出长度字段的开始偏移量
	actualLengthFieldOffset := d.LengthFieldOffset
	// 获取长度字段的值，不包括lengthAdjustment的调整值
	frameLength := d.getUnadjustedFrameLength(in, actualLengthFieldOffset, d.LengthFieldLength, d.Order)

	// 如果数据帧长度小于0，说明是个错误的数据包
	if frameLength < 0 {
		// 内部会跳过这个数据包的字节数，并抛异常
		d.failOnNegativeLengthField(in, frameLength, d.LengthFieldEndOffset)
	}

	// 套用前面的公式：长度字段后的数据字节数=长度字段的值+lengthAdjustment
	// frameLength就是长度字段的值，加上lengthAdjustment等于长度字段后的数据字节数
	// lengthFieldEndOffset为lengthFieldOffset+lengthFieldLength
	// 那说明最后计算出的frameLength就是整个数据包的长度
	frameLength += int64(d.LengthAdjustment) + int64(d.LengthFieldEndOffset)
	// 丢弃模式就是在这开启的
	// 如果数据包长度大于最大长度
	if uint64(frameLength) > d.MaxFrameLength {
		// 对超过的部分进行处理
		d.exceededFrameLength(in, frameLength)
		return nil
	}

	// 执行到这说明是正常模式
	// 数据包的大小
	frameLengthInt := int(frameLength)
	// 判断缓冲区可读字节数是否小于数据包的字节数
	if in.Len() < frameLengthInt {
		//半包，等会再来解析
		return nil
	}

	// 执行到这说明缓冲区的数据已经包含了数据包

	// 跳过的字节数是否大于数据包长度
	if d.InitialBytesToStrip > frameLengthInt {
		d.failOnFrameLengthLessThanInitialBytesToStrip(in, frameLength, d.InitialBytesToStrip)
	}
	// 跳过initialBytesToStrip个字节
	in.Next(d.InitialBytesToStrip)
	// 解码
	// 获取跳过后的真实数据长度
	actualFrameLength := frameLengthInt - d.InitialBytesToStrip
	// 提取真实的数据
	buff := make([]byte, actualFrameLength)
	_, _ = in.Read(buff)

	return buff
}

func (d *FrameDecoder) Decode(buff []byte) [][]byte {
	d.lock.Lock()
	defer d.lock.Unlock()

	d.in = append(d.in, buff...)
	resp := make([][]byte, 0)

	for {
		arr := d.decode(d.in)

		if arr != nil {
			// 证明已经解析出一个完整包
			resp = append(resp, arr)
			_size := len(arr) + d.InitialBytesToStrip
			if _size > 0 {
				d.in = d.in[_size:]
			}
		} else {
			return resp
		}
	}
}
