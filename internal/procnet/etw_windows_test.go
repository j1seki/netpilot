//go:build windows && (amd64 || arm64)

package procnet

import (
	"testing"
	"unsafe"
)

func TestETWWindowsLayout(t *testing.T) {
	var props traceProperties
	var file traceLogfile
	var record traceRecord
	if unsafe.Offsetof(props.loggerName) != 120 || unsafe.Offsetof(file.eventCallback) != 424 ||
		unsafe.Sizeof(file) != 448 || unsafe.Offsetof(record.userData) != 96 || unsafe.Offsetof(record.context) != 104 {
		t.Fatal("ETW structures do not match the Windows 64-bit ABI")
	}
}
