/**
* @File: main.go
* @Author: Jason Woo
* @Date: 2023/8/17 17:04
**/

package main

import (
	"fmt"
	"github.com/dyowoo/xnet"
	"github.com/dyowoo/xnet/examples/common"
	"os"
	"os/signal"
	"time"
)

func main() {
	l := 10
	var clientList = make(map[int]xnet.IClient, l)
	for i := 1; i <= l; i++ {
		go func(i int) {
			client := xnet.NewClient("127.0.0.1", 29001)

			client.Start()

			clientList[i] = client

			time.Sleep(time.Second * 1)

			_ = client.Conn().SendMsg(common.C2SLogin, []byte("登陆"))

			client.AddRouter(common.S2CLogin, func(request xnet.IRequest) {
				time.Sleep(time.Second * 1)

				fmt.Printf("收到服务端数据: %s\n", string(request.GetData()))
			})

		}(i)
	}

	// close
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, os.Kill)
	sig := <-c
	fmt.Println("===exit===", sig)

	for i := 1; i <= l; i++ {
		clientList[i].Stop()
	}

	time.Sleep(time.Second * 2)
}
