//go:build windows && (amd64 || arm64)

package procnet

import (
	"encoding/binary"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	traceDLL       = windows.NewLazySystemDLL("advapi32.dll")
	startTrace     = traceDLL.NewProc("StartTraceW")
	controlTrace   = traceDLL.NewProc("ControlTraceW")
	openTrace      = traceDLL.NewProc("OpenTraceW")
	closeTrace     = traceDLL.NewProc("CloseTrace")
	processTrace   = traceDLL.NewProc("ProcessTrace")
	traceTCP       = windows.GUID{Data1: 0x9a280ac0, Data2: 0xc8e0, Data3: 0x11d1, Data4: [8]byte{0x84, 0xe2, 0, 0xc0, 0x4f, 0xb9, 0x98, 0xa2}}
	traceUDP       = windows.GUID{Data1: 0xbf3a50c5, Data2: 0xa9c9, Data3: 0x4988, Data4: [8]byte{0xa0, 5, 0x2d, 0xf0, 0xb7, 0xc8, 0x0f, 0x80}}
	traceCallbacks sync.Map
	nextTraceID    atomic.Uintptr
)

// EVENT_TRACE_PROPERTIES and the trailing logger name buffer supplied to StartTraceW.
type traceProperties struct {
	size, providerID                                                                      uint32
	historicalContext, timestamp                                                          uint64
	guid                                                                                  windows.GUID
	clock, flags                                                                          uint32
	bufferSize, minimumBuffers, maximumBuffers, maximumFileSize                           uint32
	logMode, flushInterval, enableFlags                                                   uint32
	ageLimit                                                                              int32
	buffers, freeBuffers, eventsLost, buffersWritten, logBuffersLost, realTimeBuffersLost uint32
	loggerThread                                                                          uintptr
	fileNameOffset, loggerNameOffset                                                      uint32
	loggerName                                                                            [128]uint16
}

// EVENT_TRACE_LOGFILEW on 64-bit Windows. The legacy EVENT_TRACE and
// TRACE_LOGFILE_HEADER are output-only for this real-time record consumer.
type traceLogfile struct {
	fileName, loggerName           *uint16
	currentTime                    int64
	buffersRead, processMode       uint32
	currentEvent                   [88]byte
	logfileHeader                  [280]byte
	bufferCallback                 uintptr
	bufferSize, filled, eventsLost uint32
	eventCallback                  uintptr
	isKernelTrace                  uint32
	context                        uintptr
}

// EVENT_RECORD with the fields needed by the network callback.
type traceRecord struct {
	headerPrefix                    [24]byte
	provider                        windows.GUID
	id                              uint16
	version, channel, level, opcode uint8
	task                            uint16
	keyword                         uint64
	headerSuffix                    [16 + 8]byte
	bufferContext                   uint32
	extendedCount, userLength       uint16
	extendedData                    uintptr
	userData                        *byte
	context                         uintptr
}

var networkRecordCallback = windows.NewCallback(func(event *traceRecord) uintptr {
	if event == nil || event.userData == nil || event.userLength < 8 || event.version != 2 ||
		(event.provider != traceTCP && event.provider != traceUDP) {
		return 0
	}
	switch event.opcode {
	case 10, 11, 26, 27: // IPv4/IPv6 send and receive, excluding retransmits and TCP copies.
		data := unsafe.Slice(event.userData, 8)
		pid, size := binary.LittleEndian.Uint32(data), binary.LittleEndian.Uint32(data[4:])
		if pid > 4 && pid != ^uint32(0) && size > 0 {
			if cb, ok := traceCallbacks.Load(event.context); ok {
				cb.(func(int, uint32))(int(pid), size)
			}
		}
	}
	return 0
})

func startNetworkTrace(onBytes func(int, uint32)) (<-chan error, func(), error) {
	guid, err := windows.GenerateGUID()
	if err != nil {
		return nil, nil, err
	}
	name, err := windows.UTF16PtrFromString("NetPilot-Network-" + guid.String())
	if err != nil {
		return nil, nil, err
	}
	var props traceProperties
	props.size = uint32(unsafe.Sizeof(props))
	props.loggerNameOffset = uint32(unsafe.Offsetof(props.loggerName))
	props.guid, props.clock = guid, 1
	props.flags = 0x00020000                    // WNODE_FLAG_TRACED_GUID
	props.logMode = 0x100 | 0x02000000          // REAL_TIME | SYSTEM_LOGGER
	props.enableFlags = 0x00010000 | 0x10000000 // NETWORK_TCPIP | NO_SYSCONFIG
	props.flushInterval = 1

	var session uint64
	if status, _, _ := startTrace.Call(uintptr(unsafe.Pointer(&session)), uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(&props))); status != 0 {
		return nil, nil, fmt.Errorf("StartTraceW: %w", windows.Errno(status))
	}
	stopSession := func() {
		_, _, _ = controlTrace.Call(uintptr(session), uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(&props)), 1)
	}
	id := nextTraceID.Add(1)
	traceCallbacks.Store(id, onBytes)
	logfile := &traceLogfile{loggerName: name, processMode: 0x100 | 0x10000000, eventCallback: networkRecordCallback, context: id}
	handle, _, openErr := openTrace.Call(uintptr(unsafe.Pointer(logfile)))
	if handle == ^uintptr(0) {
		traceCallbacks.Delete(id)
		stopSession()
		return nil, nil, fmt.Errorf("OpenTraceW: %w", openErr)
	}

	finished := make(chan error, 1)
	go func() {
		status, _, _ := processTrace.Call(uintptr(unsafe.Pointer(&handle)), 1, 0, 0)
		runtime.KeepAlive(logfile)
		if status != 0 {
			finished <- windows.Errno(status)
		}
		close(finished)
	}()
	cleanup := func() {
		traceCallbacks.Delete(id)
		stopSession()
		_, _, _ = closeTrace.Call(handle)
	}
	return finished, cleanup, nil
}
