/**
* @File: config.go
* @Author: Jason Woo
* @Date: 2023/8/17 14:49
**/

package xnet

import (
	"encoding/json"
	"fmt"
	"github.com/dyowoo/xnet/xlog"
	"github.com/dyowoo/xnet/xutils/commandline/args"
	"github.com/dyowoo/xnet/xutils/commandline/uflag"
	"os"
	"reflect"
	"testing"
	"time"
)

const (
	ServerModeTcp       = "tcp"
	ServerModeWebsocket = "websocket"
	WorkerModeBind      = "Bind" // 为每个连接分配一个worker
)

// Config
/*
存储一切有关框架的全局参数，供其他模块使用
一些参数也可以通过 用户根据 xnet.json来配置
*/
type Config struct {
	Host              string // 当前服务器主机IP
	TCPPort           int    // 当前服务器主机监听端口号
	WsPort            int    // 当前服务器主机websocket监听端口
	Name              string // 当前服务器名称
	Version           string // 当前版本号
	MaxPacketSize     uint32 // 读写数据包的最大值
	MaxConn           int    // 当前服务器主机允许的最大链接个数
	WorkerPoolSize    uint32 // 业务工作Worker池的数量
	MaxWorkerTaskLen  uint32 // 业务工作Worker对应负责的任务队列最大任务存储数量
	WorkerMode        string // 为链接分配worker的方式
	MaxMsgChanLen     uint32 // SendBuffMsg发送消息的缓冲最大长度
	IOReadBuffSize    uint32 // 每次IO最大的读取长度
	Mode              string // "tcp":tcp监听, "websocket":websocket 监听 为空时同时开启
	LogDir            string // 日志所在文件夹 默认"./log"
	LogFile           string // 日志文件名称   默认""  --如果没有设置日志文件，打印信息将打印至stderr
	LogSaveDays       int    // 日志最大保留天数
	LogFileSize       int64  // 日志单个日志最大容量 默认 64MB,单位：字节，记得一定要换算成MB（1024 * 1024）
	LogCons           bool   // 日志标准输出  默认 false
	LogIsolationLevel int    // 日志隔离级别  -- 0：全开 1：关debug 2：关debug/info 3：关debug/info/warn ...
	HeartbeatMax      int    // 最长心跳检测间隔时间(单位：秒),超过改时间间隔，则认为超时，从配置文件读取
	CertFile          string //  证书文件名称 默认""
	PrivateKeyFile    string //  私钥文件名称 默认"" --如果没有设置证书和私钥文件，则不启用TLS加密
}

var GlobalConfig *Config

// PathExists  判断一个文件是否存在
func PathExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

// Reload 读取用户的配置文件
func (g *Config) Reload() {
	confFilePath := args.Args.ConfigFile
	if confFileExists, _ := PathExists(confFilePath); confFileExists != true {
		// 配置文件不存在也需要用默认参数初始化日志模块配置
		g.initLogConfig()

		xlog.ErrorF("config file %s is not exist!!", confFilePath)
		return
	}

	data, err := os.ReadFile(confFilePath)
	if err != nil {
		panic(err)
	}

	err = json.Unmarshal(data, g)
	if err != nil {
		panic(err)
	}

	g.initLogConfig()
}

// Show 打印配置信息
func (g *Config) Show() {
	objVal := reflect.ValueOf(g).Elem()
	objType := reflect.TypeOf(*g)

	fmt.Println("===== xnet Global Config =====")

	for i := 0; i < objVal.NumField(); i++ {
		field := objVal.Field(i)
		typeField := objType.Field(i)

		fmt.Printf("%s: %v\n", typeField.Name, field.Interface())
	}

	fmt.Println("==============================")
}

func (g *Config) HeartbeatMaxDuration() time.Duration {
	return time.Duration(g.HeartbeatMax) * time.Second
}

func (g *Config) initLogConfig() {
	if g.LogFile != "" {
		xlog.SetLogFile(g.LogDir, g.LogFile)
		xlog.SetCons(g.LogCons)
	}
	if g.LogSaveDays > 0 {
		xlog.SetMaxAge(g.LogSaveDays)
	}
	if g.LogFileSize > 0 {
		xlog.SetMaxSize(g.LogFileSize)
	}
	if g.LogIsolationLevel > xlog.LogDebug {
		xlog.SetLogLevel(g.LogIsolationLevel)
	}
}

func init() {
	pwd, err := os.Getwd()

	if err != nil {
		pwd = "."
	}

	args.InitConfigFlag(pwd+"/conf/xnet.json", "默认配置文件(/conf/fastnet.json)不存在")

	// 防止 go test 出现"flag provided but not defined: -test.panic on exit0"等错误
	testing.Init()
	uflag.Parse()

	args.FlagHandle()

	// 初始化GlobalConfig变量，设置一些默认值
	GlobalConfig = &Config{
		Name:              "xnetServerApp",
		Version:           "V1.0",
		TCPPort:           29000,
		WsPort:            28000,
		Host:              "0.0.0.0",
		MaxConn:           12000,
		MaxPacketSize:     4096,
		WorkerPoolSize:    10,
		MaxWorkerTaskLen:  1024,
		WorkerMode:        "",
		MaxMsgChanLen:     1024,
		LogDir:            pwd + "/log",
		LogFile:           "", // 默认日志文件为空，打印到stderr
		LogIsolationLevel: 0,
		HeartbeatMax:      10, // 默认心跳检测最长间隔为10秒
		IOReadBuffSize:    1024,
		CertFile:          "",
		PrivateKeyFile:    "",
		Mode:              ServerModeTcp,
	}

	// 从配置文件中加载一些用户配置的参数
	GlobalConfig.Reload()
}

// UserConfToGlobal 注意如果使用UserConf应该调用方法同步至 GlobalConfObject 因为其他参数是调用的此结构体参数
func UserConfToGlobal(config *Config) {
	// Server
	if config.Name != "" {
		GlobalConfig.Name = config.Name
	}
	if config.Host != "" {
		GlobalConfig.Host = config.Host
	}
	if config.TCPPort != 0 {
		GlobalConfig.TCPPort = config.TCPPort
	}

	// fastnet2
	if config.Version != "" {
		GlobalConfig.Version = config.Version
	}
	if config.MaxPacketSize != 0 {
		GlobalConfig.MaxPacketSize = config.MaxPacketSize
	}
	if config.MaxConn != 0 {
		GlobalConfig.MaxConn = config.MaxConn
	}
	if config.WorkerPoolSize != 0 {
		GlobalConfig.WorkerPoolSize = config.WorkerPoolSize
	}
	if config.MaxWorkerTaskLen != 0 {
		GlobalConfig.MaxWorkerTaskLen = config.MaxWorkerTaskLen
	}
	if config.WorkerMode != "" {
		GlobalConfig.WorkerMode = config.WorkerMode
	}

	if config.MaxMsgChanLen != 0 {
		GlobalConfig.MaxMsgChanLen = config.MaxMsgChanLen
	}
	if config.IOReadBuffSize != 0 {
		GlobalConfig.IOReadBuffSize = config.IOReadBuffSize
	}

	// 默认是False, config没有初始化即使用默认配置
	GlobalConfig.LogIsolationLevel = config.LogIsolationLevel
	if GlobalConfig.LogIsolationLevel > xlog.LogDebug {
		xlog.SetLogLevel(GlobalConfig.LogIsolationLevel)
	}

	// 不同于上方必填项 日志目前如果没配置应该使用默认配置
	if config.LogDir != "" {
		GlobalConfig.LogDir = config.LogDir
	}

	if config.LogFile != "" {
		GlobalConfig.LogFile = config.LogFile
		xlog.SetLogFile(GlobalConfig.LogDir, GlobalConfig.LogFile)
	}

	// Keepalive
	if config.HeartbeatMax != 0 {
		GlobalConfig.HeartbeatMax = config.HeartbeatMax
	}

	// TLS
	if config.CertFile != "" {
		GlobalConfig.CertFile = config.CertFile
	}
	if config.PrivateKeyFile != "" {
		GlobalConfig.PrivateKeyFile = config.PrivateKeyFile
	}

	if config.Mode != "" {
		GlobalConfig.Mode = config.Mode
	}
	if config.WsPort != 0 {
		GlobalConfig.WsPort = config.WsPort
	}
}
