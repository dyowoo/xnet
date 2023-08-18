/**
* @File: decoder.go
* @Author: Jason Woo
* @Date: 2023/8/17 15:58
**/

package xnet

type IDecoder interface {
	IInterceptor
	GetLengthField() *LengthField
}
