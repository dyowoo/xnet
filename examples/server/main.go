/**
* @File: main.go
* @Author: Jason Woo
* @Date: 2023/8/17 17:02
**/

package main

import (
	"encoding/json"
	"fmt"
	"github.com/dyowoo/xnet"
	"github.com/dyowoo/xnet/examples/common"
	"os"
	"os/signal"
	"time"
)

func main() {
	s := xnet.NewClient("127.0.0.1", 29000)

	s.AddRouter(common.G2STransfer, func(request xnet.IRequest) {
		var transferData common.TransferData

		if err := json.Unmarshal(request.GetData(), &transferData); err != nil {
			return
		}

		fmt.Printf("MsgID = %d, Data = %s\n", transferData.MsgID, string(transferData.Data))

		msgHandle(transferData, request)
	})

	s.Start()

	// close
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, os.Kill)
	sig := <-c
	fmt.Println("===exit===", sig)

	s.Stop()

	time.Sleep(time.Second * 2)
}

func msgHandle(transferData common.TransferData, request xnet.IRequest) {
	switch transferData.MsgID {
	case common.C2SLogin:
		transferData.MsgID = common.S2CLogin
		transferData.Data = []byte("登陆成功!")

		if data, err := json.Marshal(transferData); err == nil {
			_ = request.GetConnection().SendBuffMsg(common.S2GTransfer, data)
		}
	}
}
