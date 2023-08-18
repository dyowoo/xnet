/**
* @File: xlog.go
* @Author: Jason Woo
* @Date: 2023/8/17 14:46
**/

package xlog

var logger = NewLog("", BitDefault)

// Flags gets the flags of xlog
func Flags() int {
	return logger.Flags()
}

// ResetFlags sets the flags of xlog
func ResetFlags(flag int) {
	logger.ResetFlags(flag)
}

// AddFlag adds a flag to xlog
func AddFlag(flag int) {
	logger.AddFlag(flag)
}

// SetPrefix sets the log prefix of xlog
func SetPrefix(prefix string) {
	logger.SetPrefix(prefix)
}

// SetLogFile sets the log file of xlog
func SetLogFile(fileDir string, fileName string) {
	logger.SetLogFile(fileDir, fileName)
}

// SetMaxAge 最大保留天数
func SetMaxAge(ma int) {
	logger.SetMaxAge(ma)
}

// SetMaxSize 单个日志最大容量 单位：字节
func SetMaxSize(ms int64) {
	logger.SetMaxSize(ms)
}

// SetCons 同时输出控制台
func SetCons(b bool) {
	logger.SetConsole(b)
}

// SetLogLevel sets the log level of xlog
func SetLogLevel(logLevel int) {
	logger.SetLogLevel(logLevel)
}

func DebugF(format string, v ...interface{}) {
	logger.DebugF(format, v...)
}

func Debug(v ...interface{}) {
	logger.Debug(v...)
}

func InfoF(format string, v ...interface{}) {
	logger.InfoF(format, v...)
}

func Info(v ...interface{}) {
	logger.Info(v...)
}

func WarnF(format string, v ...interface{}) {
	logger.WarnF(format, v...)
}

func Warn(v ...interface{}) {
	logger.Warn(v...)
}

func ErrorF(format string, v ...interface{}) {
	logger.ErrorF(format, v...)
}

func Error(v ...interface{}) {
	logger.Error(v...)
}

func FatalF(format string, v ...interface{}) {
	logger.FatalF(format, v...)
}

func Fatal(v ...interface{}) {
	logger.Fatal(v...)
}

func PanicF(format string, v ...interface{}) {
	logger.PanicF(format, v...)
}

func Panic(v ...interface{}) {
	logger.Panic(v...)
}

func Stack(v ...interface{}) {
	logger.Stack(v...)
}

func init() {
	// 因为StdFastLog对象 对所有输出方法做了一层包裹，所以在打印调用函数的时候，比正常的logger对象多一层调用
	// 一般的fastLogger对象 calledDepth=2, StdFastLog的calledDepth=3
	logger.calledDepth = 3
}
