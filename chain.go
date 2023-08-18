/**
* @File: chain.go
* @Author: Jason Woo
* @Date: 2023/8/17 13:43
**/

package xnet

type IChain interface {
	Request() IcReq                             // 获取当前责任链中的请求数据(当前拦截器)
	GetIMessage() IMessage                      // 从Chain中获取IMessage
	Proceed(req IcReq) IcResp                   // 进入并执行下一个拦截器，且将请求数据传递给下一个拦截器
	ProceedWithIMessage(IMessage, IcReq) IcResp // 进入并执行下一个拦截器，且将请求数据传递给下一个拦截器
}

type Chain struct {
	req          IcReq
	position     int
	interceptors []IInterceptor
}

func NewChain(list []IInterceptor, pos int, req IcReq) IChain {
	return &Chain{
		req:          req,
		position:     pos,
		interceptors: list,
	}
}

func (c *Chain) Request() IcReq {
	return c.req
}

func (c *Chain) Proceed(request IcReq) IcResp {
	if c.position < len(c.interceptors) {
		chain := NewChain(c.interceptors, c.position+1, request)
		interceptor := c.interceptors[c.position]
		response := interceptor.Intercept(chain)
		return response
	}
	return request
}

// GetIMessage 从Chain中获取IMessage
func (c *Chain) GetIMessage() IMessage {
	req := c.Request()
	if req == nil {
		return nil
	}

	request := c.ShouldIRequest(req)
	if request == nil {
		return nil
	}

	return request.GetMessage()
}

// ProceedWithIMessage Next 通过IMessage和解码后数据进入下一个责任链任务
func (c *Chain) ProceedWithIMessage(message IMessage, response IcReq) IcResp {
	if message == nil || response == nil {
		return c.Proceed(c.Request())
	}

	req := c.Request()
	if req == nil {
		return c.Proceed(c.Request())
	}

	request := c.ShouldIRequest(req)
	if request == nil {
		return c.Proceed(c.Request())
	}

	//设置chain的request下一次请求
	request.SetResponse(response)

	return c.Proceed(request)
}

// ShouldIRequest 判断是否是IRequest
func (c *Chain) ShouldIRequest(icReq IcReq) IRequest {
	if icReq == nil {
		return nil
	}

	switch icReq.(type) {
	case IRequest:
		return icReq.(IRequest)
	default:
		return nil
	}
}
