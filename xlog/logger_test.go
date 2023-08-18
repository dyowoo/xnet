/**
* @File: logger_test.go
* @Author: Jason Woo
* @Date: 2023/8/17 14:47
**/

package xlog_test

import (
	"github.com/dyowoo/xnet/xlog"
	"testing"
)

func TestLogger(t *testing.T) {
	xlog.Info("xnet xlog")
}
