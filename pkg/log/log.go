package log

import (
	"k8s.io/klog/v2"
)

var (
	Silenced = false
)

func Logf(format string, args ...interface{}) {
	if Silenced {
		klog.Infof(format, args...)
	} else {
		klog.Errorf(format, args...)
	}
}
