/**
* @File: main.go
* @Author: Jason Woo
* @Date: 2023/8/18 10:10
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

var g xnet.IServer
var s xnet.IServer

var clients = make(map[uint64]xnet.IConnection)
var server xnet.IConnection

func main() {
	go func() {
		g = xnet.NewServer(xnet.Config{
			TCPPort: 29001,
		})

		g.SetOnConnStart(func(connection xnet.IConnection) {
			clients[connection.GetConnID()] = connection
		})

		g.SetOnConnStop(func(connection xnet.IConnection) {
			delete(clients, connection.GetConnID())
		})

		g.UseGateway(gate)

		g.Serve()
	}()

	go func() {
		s = xnet.NewServer(xnet.Config{
			TCPPort: 29000,
		})

		s.SetOnConnStart(func(connection xnet.IConnection) {
			server = connection

			fmt.Print("新服务连接\n")
		})

		s.AddRouter(common.S2GTransfer, func(request xnet.IRequest) {
			var transferData common.TransferData

			if err := json.Unmarshal(request.GetData(), &transferData); err != nil {
				return
			}

			c := clients[transferData.ConnID]

			_ = c.SendBuffMsg(transferData.MsgID, transferData.Data)
		})

		s.Serve()
	}()

	// close
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, os.Kill)
	sig := <-c
	fmt.Println("===exit===", sig)

	g.Stop()
	s.Stop()

	time.Sleep(time.Second * 2)
}

func gate(request xnet.IRequest) {
	fmt.Printf("收到客户端消息: msgID = %d, data = %s\n", request.GetMsgID(), string(request.GetData()))

	transferData := common.TransferData{
		ConnID: request.GetConnection().GetConnID(),
		MsgID:  request.GetMsgID(),
		Data:   request.GetData(),
	}

	if data, err := json.Marshal(transferData); err == nil {
		_ = server.SendBuffMsg(common.G2STransfer, data)
	}
}
