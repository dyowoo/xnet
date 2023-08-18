/**
* @File: interceptor.go
* @Author: Jason Woo
* @Date: 2023/8/17 13:42
**/

package xnet

// IcReq 拦截器输入数据
type IcReq any

// IcResp 拦截器输出数据
type IcResp any

type IInterceptor interface {
	Intercept(IChain) IcResp // 拦截器的拦截处理方法,由开发者定义
}
