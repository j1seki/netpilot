//go:build !linux && (!windows || (!amd64 && !arm64))

package procnet

import "context"

func (m *TrafficMonitor) run(context.Context) {
	m.publish(TrafficSnapshot{Detail: "Host-wide process traffic measurement is unavailable on this platform."})
}
