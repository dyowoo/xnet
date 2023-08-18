/**
* @File: delay_func.go
* @Author: Jason Woo
* @Date: 2023/8/17 16:58
**/

package xtimer

import (
	"fmt"
	"github.com/dyowoo/xnet/xlog"
	"reflect"
)

type DelayFunc struct {
	f    func(...any)
	args []interface{}
}

func NewDelayFunc(f func(v ...interface{}), args ...interface{}) *DelayFunc {
	return &DelayFunc{
		f:    f,
		args: args,
	}
}

func (d *DelayFunc) String() string {
	return fmt.Sprintf("{func: %s, args: %s}", reflect.TypeOf(d.f).Name(), d.args)
}

func (d *DelayFunc) Call() {
	defer func() {
		if err := recover(); err != nil {
			xlog.ErrorF("%s call err: %v", d.String(), err)
		}
	}()

	d.f(d.args...)
}
