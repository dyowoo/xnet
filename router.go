/**
* @File: handlers.go
* @Author: Jason Woo
* @Date: 2023/8/17 13:27
**/

package xnet

import (
	"strconv"
	"sync"
)

type RouterHandler func(request IRequest)

type IRouter interface {
	Use(...RouterHandler)                       // 添加全局组件
	AddHandler(uint32, ...RouterHandler)        // 添加业务处理器集合
	GetHandlers(uint32) ([]RouterHandler, bool) // 获得当前的所有注册在MsgId的处理器集合
}

type Router struct {
	apis     map[uint32][]RouterHandler
	handlers []RouterHandler
	sync.RWMutex
}

func NewRouter() *Router {
	return &Router{
		apis:     make(map[uint32][]RouterHandler, 10),
		handlers: make([]RouterHandler, 0, 6),
	}
}

func (r *Router) Use(handlers ...RouterHandler) {
	r.handlers = append(r.handlers, handlers...)
}

func (r *Router) AddHandler(msgID uint32, handlers ...RouterHandler) {
	if _, ok := r.apis[msgID]; ok {
		panic("repeated api, msgID = " + strconv.Itoa(int(msgID)))
	}

	size := len(r.handlers) + len(handlers)

	mergeHandlers := make([]RouterHandler, size)
	copy(mergeHandlers, r.handlers)
	copy(mergeHandlers[len(r.handlers):], handlers)

	r.apis[msgID] = append(r.apis[msgID], mergeHandlers...)
}

func (r *Router) GetHandlers(msgID uint32) ([]RouterHandler, bool) {
	r.RLock()
	defer r.RUnlock()

	handlers, ok := r.apis[msgID]

	return handlers, ok
}
