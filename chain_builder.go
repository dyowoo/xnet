/**
* @File: chain_builder.go
* @Author: Jason Woo
* @Date: 2023/8/17 13:56
**/

package xnet

type chainBuilder struct {
	body       []IInterceptor
	head, tail IInterceptor
}

func newChainBuilder() *chainBuilder {
	return &chainBuilder{
		body: make([]IInterceptor, 0),
	}
}

// Head adds an interceptor to the head of the chain.
func (ic *chainBuilder) Head(interceptor IInterceptor) {
	ic.head = interceptor
}

// Tail adds an interceptor to the tail of the chain.
func (ic *chainBuilder) Tail(interceptor IInterceptor) {
	ic.tail = interceptor
}

// AddInterceptor adds an interceptor to the body of the chain.
func (ic *chainBuilder) AddInterceptor(interceptor IInterceptor) {
	ic.body = append(ic.body, interceptor)
}

// Execute executes all the interceptors in the current chain in order.
func (ic *chainBuilder) Execute(req IcReq) IcResp {

	// Put all the interceptors into the builder
	var interceptors []IInterceptor

	if ic.head != nil {
		interceptors = append(interceptors, ic.head)
	}

	if len(ic.body) > 0 {
		interceptors = append(interceptors, ic.body...)
	}

	if ic.tail != nil {
		interceptors = append(interceptors, ic.tail)
	}

	// Create a new interceptor chain and execute each interceptor
	chain := NewChain(interceptors, 0, req)

	// Execute the chain
	return chain.Proceed(req)
}
