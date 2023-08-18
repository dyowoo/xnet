/**
* @File: logger_core.go
* @Author: Jason Woo
* @Date: 2023/8/17 14:36
**/

package xlog

import (
	"bytes"
	"context"
	"fmt"
	"github.com/dyowoo/xnet/xutils"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)

type ILogger interface {
	InfoF(format string, v ...interface{})
	ErrorF(format string, v ...interface{})
	DebugF(format string, v ...interface{})
	InfoFX(ctx context.Context, format string, v ...interface{})
	ErrorFX(ctx context.Context, format string, v ...interface{})
	DebugFX(ctx context.Context, format string, v ...interface{})
}

const (
	LogMaxBuf = 1024 * 1024
)

// 日志头部信息标记位，采用bitmap方式，用户可以选择头部需要哪些标记位被打印
const (
	BitDate         = 1 << iota                            // Date flag bit 2019/01/23 (日期标记位)
	BitTime                                                // Time flag bit 01:23:12 (时间标记位)
	BitMicroSeconds                                        // Microsecond flag bit 01:23:12.111222 (微秒级标记位)
	BitLongFile                                            // Complete file name /home/go/src/fastnet2/server.go (完整文件名称)
	BitShortFile                                           // Last file name server.go (最后文件名)
	BitLevel                                               // Current log level: 0(Debug), 1(Info), 2(Warn), 3(Error), 4(Panic), 5(Fatal) (当前日志级别)
	BitStdFlag      = BitDate | BitTime                    // Standard log header format (标准头部日志格式)
	BitDefault      = BitLevel | BitShortFile | BitStdFlag // Default log header format (默认日志头部格式)
)

// Log Level
const (
	LogDebug = iota
	LogInfo
	LogWarn
	LogError
	LogPanic
	LogFatal
)

// Log Level String
var levels = []string{
	"[DEBUG]",
	"[INFO]",
	"[WARN]",
	"[ERROR]",
	"[PANIC]",
	"[FATAL]",
}

type LoggerCore struct {
	mu             sync.Mutex   // 确保多协程读写文件，防止文件内容混乱，做到协程安全
	prefix         string       // 每行log日志的前缀字符串,拥有日志标记
	flag           int          // 日志标记位
	buf            bytes.Buffer // 输出的缓冲区
	isolationLevel int          // 日志隔离级别
	calledDepth    int          // 获取日志文件名和代码上述的runtime.Call 的函数调用层数
	fw             *xutils.Writer
	onLogHook      func([]byte)
}

func NewLog(prefix string, flag int) *LoggerCore {
	// 默认 debug打开， calledDepth深度为2,FastLogger对象调用日志打印方法最多调用两层到达output函数
	log := &LoggerCore{prefix: prefix, flag: flag, isolationLevel: 0, calledDepth: 2}
	// 设置log对象 回收资源 析构方法(不设置也可以，go的Gc会自动回收，强迫症没办法)
	runtime.SetFinalizer(log, CleanLog)

	return log
}

// CleanLog Recycle log resources
func CleanLog(log *LoggerCore) {
	log.closeFile()
}

func (log *LoggerCore) SetLogHook(f func([]byte)) {
	log.onLogHook = f
}

/*
formatHeader generates the header information for a log entry.

t: The current time.
file: The file name of the source code invoking the log function.
line: The line number of the source code invoking the log function.
level: The log level of the current log entry.
*/
func (log *LoggerCore) formatHeader(t time.Time, file string, line int, level int) {
	var buf = &log.buf
	// If the current prefix string is not empty, write the prefix first.
	if log.prefix != "" {
		buf.WriteByte('<')
		buf.WriteString(log.prefix)
		buf.WriteByte('>')
	}

	// If the time-related flags are set, add the time information to the log header.
	if log.flag&(BitDate|BitTime|BitMicroSeconds) != 0 {
		// Date flag is set
		if log.flag&BitDate != 0 {
			year, month, day := t.Date()
			itoa(buf, year, 4)
			buf.WriteByte('/') // "2019/"
			itoa(buf, int(month), 2)
			buf.WriteByte('/') // "2019/04/"
			itoa(buf, day, 2)
			buf.WriteByte(' ') // "2019/04/11 "
		}

		// Time flag is set
		if log.flag&(BitTime|BitMicroSeconds) != 0 {
			hour, minute, sec := t.Clock()
			itoa(buf, hour, 2)
			buf.WriteByte(':') // "11:"
			itoa(buf, minute, 2)
			buf.WriteByte(':') // "11:15:"
			itoa(buf, sec, 2)  // "11:15:33"
			// Microsecond flag is set
			if log.flag&BitMicroSeconds != 0 {
				buf.WriteByte('.')
				itoa(buf, t.Nanosecond()/1e3, 6) // "11:15:33.123123
			}
			buf.WriteByte(' ')
		}

		// Log level flag is set
		if log.flag&BitLevel != 0 {
			buf.WriteString(levels[level] + "\t")
		}

		// Short file name flag or long file name flag is set
		if log.flag&(BitShortFile|BitLongFile) != 0 {
			// Short file name flag is set
			if log.flag&BitShortFile != 0 {
				short := file
				for i := len(file) - 1; i > 0; i-- {
					if file[i] == '/' {
						// Get the file name after the last '/' character, e.g. "fastnet2.go" from "/home/go/src/fastnet2.go"
						short = file[i+1:]
						break
					}
				}
				file = short
			}
			buf.WriteString(file)
			buf.WriteByte(':')
			itoa(buf, line, -1) // line number
			buf.WriteString(":\t")
		}
	}
}

// OutPut outputs log file, the original method
func (log *LoggerCore) OutPut(level int, s string) error {
	now := time.Now() // get current time
	var file string   // file name of the current caller of the log interface
	var line int      // line number of the executed code
	log.mu.Lock()
	defer log.mu.Unlock()

	if log.flag&(BitShortFile|BitLongFile) != 0 {
		log.mu.Unlock()
		var ok bool
		// get the file name and line number of the current caller
		_, file, line, ok = runtime.Caller(log.calledDepth)
		if !ok {
			file = "unknown-file"
			line = 0
		}
		log.mu.Lock()
	}

	// reset buffer
	log.buf.Reset()
	// write log header
	log.formatHeader(now, file, line, level)
	// write log content
	log.buf.WriteString(s)
	// add line break
	if len(s) > 0 && s[len(s)-1] != '\n' {
		log.buf.WriteByte('\n')
	}

	var err error
	if log.fw == nil {
		// if log file is not set, output to console
		_, _ = os.Stderr.Write(log.buf.Bytes())
	} else {
		// write the filled buffer to IO output
		_, err = log.fw.Write(log.buf.Bytes())
	}

	if log.onLogHook != nil {
		log.onLogHook(log.buf.Bytes())
	}
	return err
}

func (log *LoggerCore) verifyLogIsolation(logLevel int) bool {
	if log.isolationLevel > logLevel {
		return true
	} else {
		return false
	}
}

func (log *LoggerCore) DebugF(format string, v ...interface{}) {
	if log.verifyLogIsolation(LogDebug) {
		return
	}
	_ = log.OutPut(LogDebug, fmt.Sprintf(format, v...))
}

func (log *LoggerCore) Debug(v ...interface{}) {
	if log.verifyLogIsolation(LogDebug) {
		return
	}
	_ = log.OutPut(LogDebug, fmt.Sprintln(v...))
}

func (log *LoggerCore) InfoF(format string, v ...interface{}) {
	if log.verifyLogIsolation(LogInfo) {
		return
	}
	_ = log.OutPut(LogInfo, fmt.Sprintf(format, v...))
}

func (log *LoggerCore) Info(v ...interface{}) {
	if log.verifyLogIsolation(LogInfo) {
		return
	}
	_ = log.OutPut(LogInfo, fmt.Sprintln(v...))
}

func (log *LoggerCore) WarnF(format string, v ...interface{}) {
	if log.verifyLogIsolation(LogWarn) {
		return
	}
	_ = log.OutPut(LogWarn, fmt.Sprintf(format, v...))
}

func (log *LoggerCore) Warn(v ...interface{}) {
	if log.verifyLogIsolation(LogWarn) {
		return
	}
	_ = log.OutPut(LogWarn, fmt.Sprintln(v...))
}

func (log *LoggerCore) ErrorF(format string, v ...interface{}) {
	if log.verifyLogIsolation(LogError) {
		return
	}
	_ = log.OutPut(LogError, fmt.Sprintf(format, v...))
}

func (log *LoggerCore) Error(v ...interface{}) {
	if log.verifyLogIsolation(LogError) {
		return
	}
	_ = log.OutPut(LogError, fmt.Sprintln(v...))
}

func (log *LoggerCore) FatalF(format string, v ...interface{}) {
	if log.verifyLogIsolation(LogFatal) {
		return
	}
	_ = log.OutPut(LogFatal, fmt.Sprintf(format, v...))
	os.Exit(1)
}

func (log *LoggerCore) Fatal(v ...interface{}) {
	if log.verifyLogIsolation(LogFatal) {
		return
	}
	_ = log.OutPut(LogFatal, fmt.Sprintln(v...))
	os.Exit(1)
}

func (log *LoggerCore) PanicF(format string, v ...interface{}) {
	if log.verifyLogIsolation(LogPanic) {
		return
	}
	s := fmt.Sprintf(format, v...)
	_ = log.OutPut(LogPanic, s)
	panic(s)
}

func (log *LoggerCore) Panic(v ...interface{}) {
	if log.verifyLogIsolation(LogPanic) {
		return
	}
	s := fmt.Sprintln(v...)
	_ = log.OutPut(LogPanic, s)
	panic(s)
}

func (log *LoggerCore) Stack(v ...interface{}) {
	s := fmt.Sprint(v...)
	s += "\n"
	buf := make([]byte, LogMaxBuf)
	n := runtime.Stack(buf, true) //得到当前堆栈信息
	s += string(buf[:n])
	s += "\n"
	_ = log.OutPut(LogError, s)
}

// Flags 获取当前日志bitmap标记
func (log *LoggerCore) Flags() int {
	log.mu.Lock()
	defer log.mu.Unlock()
	return log.flag
}

// ResetFlags 重新设置日志Flags bitMap 标记位
func (log *LoggerCore) ResetFlags(flag int) {
	log.mu.Lock()
	defer log.mu.Unlock()
	log.flag = flag
}

// AddFlag 添加flag标记
func (log *LoggerCore) AddFlag(flag int) {
	log.mu.Lock()
	defer log.mu.Unlock()
	log.flag |= flag
}

// SetPrefix 设置日志的 用户自定义前缀字符串
func (log *LoggerCore) SetPrefix(prefix string) {
	log.mu.Lock()
	defer log.mu.Unlock()
	log.prefix = prefix
}

// SetLogFile 设置日志文件输出
func (log *LoggerCore) SetLogFile(fileDir string, fileName string) {
	if log.fw != nil {
		_ = log.fw.Close()
	}
	log.fw = xutils.New(filepath.Join(fileDir, fileName))
}

// SetMaxAge 最大保留天数
func (log *LoggerCore) SetMaxAge(ma int) {
	if log.fw == nil {
		return
	}
	log.mu.Lock()
	defer log.mu.Unlock()
	log.fw.SetMaxAge(ma)
}

// SetMaxSize 单个日志最大容量 单位：字节
func (log *LoggerCore) SetMaxSize(ms int64) {
	if log.fw == nil {
		return
	}
	log.mu.Lock()
	defer log.mu.Unlock()
	log.fw.SetMaxSize(ms)
}

// SetConsole 同时输出控制台
func (log *LoggerCore) SetConsole(b bool) {
	if log.fw == nil {
		return
	}
	log.mu.Lock()
	defer log.mu.Unlock()
	log.fw.SetCons(b)
}

// 关闭日志绑定的文件
func (log *LoggerCore) closeFile() {
	if log.fw != nil {
		_ = log.fw.Close()
	}
}

func (log *LoggerCore) SetLogLevel(logLevel int) {
	log.isolationLevel = logLevel
}

// 将一个整形转换成一个固定长度的字符串，字符串宽度应该是大于0的
// 要确保buffer是有容量空间的
func itoa(buf *bytes.Buffer, i int, wID int) {
	var u = uint(i)
	if u == 0 && wID <= 1 {
		buf.WriteByte('0')
		return
	}

	// Assemble decimal in reverse order.
	var b [32]byte
	bp := len(b)
	for ; u > 0 || wID > 0; u /= 10 {
		bp--
		wID--
		b[bp] = byte(u%10) + '0'
	}

	// avoID slicing b to avoID an allocation.
	for bp < len(b) {
		buf.WriteByte(b[bp])
		bp++
	}
}
