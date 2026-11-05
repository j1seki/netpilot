//go:build windows

package apps

import (
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

func (m *Manager) discoverWindows() {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return
	}
	defer windows.CloseHandle(snapshot)

	var entry windows.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))

	seen := make(map[int]bool)
	err = windows.Process32First(snapshot, &entry)
	for err == nil {
		pid := int(entry.ProcessID)
		if pid > 4 { // Skip System (PID 4) and Idle (PID 0)
			name := syscall.UTF16ToString(entry.ExeFile[:])
			if name != "" {
				seen[pid] = true
				if p, ok := m.processes[pid]; ok {
					p.LastActive = time.Now()
				} else {
					m.processes[pid] = &ProcessInfo{
						PID:         pid,
						Name:        name,
						Executable:  name,
						CommandLine: name,
						IconHint:    inferIconHint(name),
						AssignedNIC: "Ethernet",
						LastActive:  time.Now(),
					}
				}
			}
		}
		err = windows.Process32Next(snapshot, &entry)
	}

	for pid := range m.processes {
		if !seen[pid] {
			delete(m.processes, pid)
		}
	}
}
