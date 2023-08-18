/**
* @File: default_router_func.go
* @Author: Jason Woo
* @Date: 2023/8/17 16:33
**/

package xnet

import (
	"bytes"
	"fmt"
	"github.com/dyowoo/xnet/xlog"
	"path"
	"runtime"
	"strings"
	"time"
)

const (
	StackBegin = 3 // 开始追踪堆栈信息的层数
	StackEnd   = 5 // 追踪到最后的层数
)

// RouterRecovery
// 用来存放一些RouterSlicesMode下的路由可用的默认中间件
// 如果使用NewDefaultRouterSlicesServer方法初始化的获得的server将自带这个函数
// 作用是接收业务执行上产生的panic并且尝试记录现场信息
func RouterRecovery(request IRequest) {
	defer func() {
		if err := recover(); err != nil {
			panicInfo := getInfo(StackBegin)
			xlog.ErrorF("msgId:%d handler panic: info:%s err:%v", request.GetMsgID(), panicInfo, err)
		}

	}()

	request.RouterNext()
}

// RouterTime 简单累计所有路由组的耗时，不启用
func RouterTime(request IRequest) {
	now := time.Now()
	request.RouterNext()
	duration := time.Since(now)
	fmt.Println(duration.String())
}

func getInfo(ship int) string {
	info := new(bytes.Buffer)

	// 也可以不指定终点层数即i := ship;; i++ 通过if！ok 结束循环，但是会一直追到最底层报错信息
	for i := ship; i <= StackEnd; i++ {
		pc, file, lineNo, ok := runtime.Caller(i)
		if !ok {
			break
		}
		funcName := runtime.FuncForPC(pc).Name()
		fileName := path.Base(file)
		funcName = strings.Split(funcName, ".")[1]

		_, _ = fmt.Fprintf(info, "funcName:%s fileName:%s LineNo:%d\n", funcName, fileName, lineNo)
	}

	return info.String()

}
