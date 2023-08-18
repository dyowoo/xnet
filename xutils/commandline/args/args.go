/**
* @File: args.go
* @Author: Jason Woo
* @Date: 2023/8/17 14:54
**/

package args

import (
	"github.com/dyowoo/xnet/xutils/commandline/uflag"
	"os"
	"path/filepath"
)

type args struct {
	ExeAbsDir  string
	ExeName    string
	ConfigFile string
}

var (
	Args   = args{}
	isInit = false
)

func init() {
	exe := os.Args[0]

	pwd, err := os.Getwd()
	if err != nil {
		panic(err)
	}

	Args.ExeAbsDir = pwd
	Args.ExeName = filepath.Base(exe)
}

func InitConfigFlag(defaultValue string, tips string) {
	if isInit {
		return
	}
	isInit = true

	uflag.StringVar(&Args.ConfigFile, "c", defaultValue, tips)
	return
}

func FlagHandle() {
	filePath, err := filepath.Abs(Args.ConfigFile)
	if err != nil {
		panic(err)
	}
	Args.ConfigFile = filePath
}
