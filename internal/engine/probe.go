package engine

import (
	"net"
	"time"

	"netpilot/internal/nics"
)

// Internet reachability probes: a TCP connect to a well-known anycast IP, bound to the NIC.
var probeTargetsDefault = [2][]string{
	Fam4: {"1.1.1.1:443", "8.8.8.8:443"},
	Fam6: {"[2606:4700:4700::1111]:443", "[2001:4860:4860::8888]:443"},
}

const (
	probeTimeout   = 2 * time.Second
	probeEveryUp   = 15 * time.Second
	probeEveryDown = 4 * time.Second
)

func probe(addr *slotAddr, f Family, targets []string) (bool, time.Duration) {
	ip := addr.ip(f)
	if ip == nil {
		return false, 0
	}
	network := "tcp4"
	if f == Fam6 {
		network = "tcp6"
	}
	for _, t := range targets {
		d := net.Dialer{LocalAddr: &net.TCPAddr{IP: ip}, Control: nics.ForceEgress(addr.index, f == Fam6), Timeout: probeTimeout}
		start := time.Now()
		if c, err := d.Dial(network, t); err == nil {
			rtt := time.Since(start)
			c.Close()
			return true, rtt
		}
	}
	return false, 0
}

type probeJob struct {
	sl   *Slot
	f    Family
	addr *slotAddr
}

// RunProbes checks every adapter's internet access per address family: new ones immediately,
// healthy ones occasionally, dead ones more often so they rejoin quickly. A tether with no data
// or a captive-portal Wi-Fi therefore never receives flows, and a recovered link is used again
// within seconds. Runs until stop is closed (nil = forever).
func (s *Scheduler) RunProbes(stop <-chan struct{}) {
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
		}
		for _, j := range s.probeDue() {
			go func() {
				targets := s.probeTargets(j.f)
				up, rtt := probe(j.addr, j.f, targets)
				s.finishProbe(j, up, rtt)
			}()
		}
	}
}

// probeDue returns the (slot, family) pairs that need a probe now and marks them in flight.
func (s *Scheduler) probeDue() []*probeJob {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	var due []*probeJob
	for _, sl := range s.slots {
		if !sl.present {
			continue
		}
		for _, f := range []Family{Fam4, Fam6} {
			if sl.ip(f) == nil || sl.probing[f] != nil {
				continue
			}
			every := probeEveryUp
			switch sl.state[f] {
			case stateUnknown:
				every = 0
			case stateDown:
				every = probeEveryDown
			}
			if now.Sub(sl.lastProbe[f]) >= every {
				job := &probeJob{sl: sl, f: f, addr: sl.addr.Load()}
				sl.lastProbe[f] = now
				sl.probing[f] = job
				due = append(due, job)
			}
		}
	}
	return due
}

func (s *Scheduler) finishProbe(job *probeJob, up bool, rtt time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sl, f := job.sl, job.f
	if sl.probing[f] == job {
		sl.probing[f] = nil
		if sl.present {
			s.recordProbeLocked(sl, f, up, rtt)
		}
	}
}
