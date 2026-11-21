package engine

import (
	"net"
	"strconv"
	"sync"
	"time"

	"golang.org/x/net/dns/dnsmessage"
)

// ProcessActivity is forwarding metadata for the selected process. The
// inspector never retains packet bodies or TLS application data.
type ProcessActivity struct {
	At          time.Time `json:"at"`
	Protocol    string    `json:"protocol"`
	Destination string    `json:"destination"`
	Adapter     string    `json:"adapter"`
	RuleID      string    `json:"ruleId,omitempty"`
	DNSQuestion string    `json:"dnsQuestion,omitempty"`
}

type processInspection struct {
	mu      sync.Mutex
	pid     int
	expires time.Time
	recent  []ProcessActivity
}

// InspectProcess selects one PID for short-lived, in-memory observation.
// Calling it again refreshes the observation while the inspector is open.
func (b *Balancer) InspectProcess(pid int) []ProcessActivity {
	ins := &b.processInspection
	ins.mu.Lock()
	defer ins.mu.Unlock()
	now := time.Now()
	if ins.pid != pid || !now.Before(ins.expires) {
		ins.recent = nil
	}
	ins.pid = pid
	ins.expires = now.Add(12 * time.Second)
	b.inspectedPID.Store(int64(pid))
	out := make([]ProcessActivity, len(ins.recent))
	for i := range ins.recent {
		out[i] = ins.recent[len(out)-1-i]
	}
	return out
}

func (b *Balancer) recordProcessTCP(pid int, host string, port int, adapter, ruleID string) {
	if pid > 0 && b.inspectedPID.Load() == int64(pid) {
		b.recordActivity(pid, ProcessActivity{
			Protocol: "TCP", Destination: net.JoinHostPort(host, strconv.Itoa(port)), Adapter: adapter, RuleID: ruleID,
		})
	}
}

func (b *Balancer) recordProcessUDP(pid int, addr net.Addr, adapter, ruleID string, data []byte) {
	if pid <= 0 || b.inspectedPID.Load() != int64(pid) || addr == nil {
		return
	}
	entry := ProcessActivity{Protocol: "UDP", Destination: addr.String(), Adapter: adapter, RuleID: ruleID}
	if udp, ok := addr.(*net.UDPAddr); ok && udp.Port == 53 {
		var parser dnsmessage.Parser
		if header, err := parser.Start(data); err == nil && !header.Response {
			if question, err := parser.Question(); err == nil {
				entry.DNSQuestion = question.Name.String() + " " + question.Type.String()
			}
		}
	}
	b.recordActivity(pid, entry)
}

func (b *Balancer) recordActivity(pid int, entry ProcessActivity) {
	ins := &b.processInspection
	ins.mu.Lock()
	defer ins.mu.Unlock()
	if ins.pid != pid {
		return
	}
	entry.At = time.Now()
	if !entry.At.Before(ins.expires) {
		ins.recent = nil
		b.inspectedPID.Store(0)
		return
	}
	for i, old := range ins.recent {
		if old.Protocol == entry.Protocol && old.Destination == entry.Destination && old.Adapter == entry.Adapter && old.DNSQuestion == entry.DNSQuestion && old.RuleID == entry.RuleID {
			copy(ins.recent[i:], ins.recent[i+1:])
			ins.recent[len(ins.recent)-1] = entry
			return
		}
	}
	const maxEntries = 64
	if len(ins.recent) == maxEntries {
		copy(ins.recent, ins.recent[1:])
		ins.recent[maxEntries-1] = entry
		return
	}
	ins.recent = append(ins.recent, entry)
}
