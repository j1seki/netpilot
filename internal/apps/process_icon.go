package apps

import (
	"path/filepath"
	"sync"
)

// Icon returns a cached PNG for a discovered process. The requested executable
// must still match the PID, so this is not a general-purpose file endpoint.
func (m *Manager) Icon(pid int, executable string) []byte {
	m.mu.RLock()
	p := m.processes[pid]
	valid := p != nil && p.Executable == executable && filepath.IsAbs(executable)
	m.mu.RUnlock()
	if !valid {
		return nil
	}
	loader := sync.OnceValue(func() []byte { return nativeIcon(executable) })
	cached, _ := m.icons.LoadOrStore(executable, loader)
	return cached.(func() []byte)()
}
