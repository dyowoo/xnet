/**
* @File: msg.go
* @Author: Jason Woo
* @Date: 2023/8/18 16:12
**/

package common

const (
	G2STransfer = 100001
	S2GTransfer = 100002

	C2SLogin = 100
	S2CLogin = 101
)

type TransferData struct {
	ConnID uint64
	MsgID  uint32
	Data   []byte
}
