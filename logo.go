/**
* @File: logo.go
* @Author: Jason Woo
* @Date: 2023/8/17 16:01
**/

package xnet

import "fmt"

var logo = `
   ████                     ██                      ██  
  ░██░                     ░██                     ░██  
 ██████  ██████    ██████ ██████ ███████   █████  ██████
░░░██░  ░░░░░░██  ██░░░░ ░░░██░ ░░██░░░██ ██░░░██░░░██░ 
  ░██    ███████ ░░█████   ░██   ░██  ░██░███████  ░██  
  ░██   ██░░░░██  ░░░░░██  ░██   ░██  ░██░██░░░░   ░██  
  ░██  ░░████████ ██████   ░░██  ███  ░██░░██████  ░░██ 
  ░░    ░░░░░░░░ ░░░░░░     ░░  ░░░   ░░  ░░░░░░    ░░  `

func PrintLogo() {
	fmt.Println(logo)
	fmt.Printf("\n[XNet] Version: %s, MaxConn: %d, MaxPacketSize: %d\n",
		GlobalConfig.Version,
		GlobalConfig.MaxConn,
		GlobalConfig.MaxPacketSize)
}
