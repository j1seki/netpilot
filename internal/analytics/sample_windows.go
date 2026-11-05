//go:build windows

package analytics

import (
	"net"
	"syscall"

	"golang.org/x/sys/windows"
)

func (c *Collector) sampleWindowsNet() error {
	ifaces, err := net.Interfaces()
	if err != nil {
		return err
	}

	for _, ifc := range ifaces {
		if ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		var row windows.MibIfRow2
		row.InterfaceIndex = uint32(ifc.Index)
		if err := windows.GetIfEntry2Ex(0, &row); err == nil {
			name := syscall.UTF16ToString(row.Alias[:])
			if name == "" {
				name = ifc.Name
			}
			c.RecordNICSample(name, int64(row.InOctets), int64(row.OutOctets))
		}
	}
	c.PushHistoryPoint()
	return nil
}
