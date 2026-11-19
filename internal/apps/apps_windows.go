//go:build windows

package apps

import (
	"path/filepath"
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
	previousCPU := m.cpu
	m.cpu = make(map[int]cpuSample)
	now := time.Now()
	err = windows.Process32First(snapshot, &entry)
	for err == nil {
		pid := int(entry.ProcessID)
		if pid > 4 { // Skip System (PID 4) and Idle (PID 0)
			name := syscall.UTF16ToString(entry.ExeFile[:])
			if name != "" {
				seen[pid] = true
				if p, ok := m.processes[pid]; ok && p.Name == name && filepath.IsAbs(p.Executable) {
					p.LastActive = time.Now()
				} else {
					m.processes[pid] = &ProcessInfo{
						PID:         pid,
						Name:        name,
						Executable:  windowsProcessPath(pid, name),
						CommandLine: name,
						IconHint:    inferIconHint(name),
						AssignedNIC: "Ethernet",
						LastActive:  time.Now(),
					}
				}
				m.sampleWindowsResources(m.processes[pid], previousCPU[pid], now)
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

var getProcessMemoryInfo = windows.NewLazySystemDLL("psapi.dll").NewProc("GetProcessMemoryInfo")

func (m *Manager) sampleWindowsResources(p *ProcessInfo, previous cpuSample, now time.Time) {
	p.CPUPercent, p.MemoryBytes = nil, nil
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(p.PID))
	if err != nil {
		return
	}
	defer windows.CloseHandle(h)
	var creation, exit, kernel, user windows.Filetime
	if windows.GetProcessTimes(h, &creation, &exit, &kernel, &user) == nil {
		ticks := func(ft windows.Filetime) uint64 { return uint64(ft.HighDateTime)<<32 | uint64(ft.LowDateTime) }
		s := cpuSample{created: ticks(creation), ticks: ticks(kernel) + ticks(user), at: now}
		m.cpu[p.PID] = s
		p.CPUPercent = s.usage(previous)
	}
	var counters struct {
		Size, PageFaults                                                                                                 uint32
		PeakWorkingSet, WorkingSet, QuotaPeakPaged, QuotaPaged, QuotaPeakNonPaged, QuotaNonPaged, Pagefile, PeakPagefile uintptr
	}
	counters.Size = uint32(unsafe.Sizeof(counters))
	ok, _, _ := getProcessMemoryInfo.Call(uintptr(h), uintptr(unsafe.Pointer(&counters)), uintptr(counters.Size))
	if ok == 0 {
		if readHandle, err := windows.OpenProcess(windows.PROCESS_QUERY_INFORMATION|windows.PROCESS_VM_READ, false, uint32(p.PID)); err == nil {
			ok, _, _ = getProcessMemoryInfo.Call(uintptr(readHandle), uintptr(unsafe.Pointer(&counters)), uintptr(counters.Size))
			windows.CloseHandle(readHandle)
		}
	}
	if ok != 0 {
		bytes := uint64(counters.WorkingSet)
		p.MemoryBytes = &bytes
	}
}

func windowsProcessPath(pid int, fallback string) string {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return fallback
	}
	defer windows.CloseHandle(h)
	var path [32768]uint16
	size := uint32(len(path))
	if windows.QueryFullProcessImageName(h, 0, &path[0], &size) != nil {
		return fallback
	}
	return windows.UTF16ToString(path[:size])
}
