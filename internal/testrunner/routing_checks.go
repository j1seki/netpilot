package testrunner

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"netpilot/internal/engine"
	"netpilot/internal/nics"
)

type routingTarget struct {
	host  string
	port  int
	links []nics.Nic
}

func liveRoutingChecks(tetherAdapter string) []inProcTest {
	target := sync.OnceValues(func() (routingTarget, error) { return findLiveRoutingTarget(tetherAdapter) })
	return []inProcTest{
		{
			name: "Speed mode over live adapters", classification: "System",
			fn: func() (TestStatus, string) {
				plan, err := target()
				if err != nil {
					return StatusBlocked, err.Error()
				}
				return checkSpeed(plan)
			},
		},
		{
			name: "Live link failover and recovery", classification: "System",
			affectsConnection: true,
			disruptionWarning: "The diagnostic engine loses its primary link and opens traffic through the backup. The desktop's routes remain in place.",
			fn: func() (TestStatus, string) {
				plan, err := target()
				if err != nil {
					return StatusBlocked, err.Error()
				}
				return checkLiveFailover(plan, tetherAdapter)
			},
		},
	}
}

func findLiveRoutingTarget(tetherAdapter string) (routingTarget, error) {
	found, _, err := nics.DiscoverNicsDetailed(nil, nil)
	if err != nil {
		return routingTarget{}, fmt.Errorf("discover eligible adapters: %w", err)
	}
	var eligible []nics.Nic
	for _, nic := range found {
		if nic.IP.To4() != nil {
			nic.Weight = 1
			eligible = append(eligible, nic)
		}
	}
	if len(eligible) < 2 {
		return routingTarget{}, fmt.Errorf("two eligible IPv4 adapters are required; NetPilot discovered %d", len(eligible))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	for _, host := range []string{"1.1.1.1", "8.8.8.8"} {
		// Probe in parallel and keep order from discovery for a stable primary.
		ready := make([]bool, len(eligible))
		var group sync.WaitGroup
		for i, adapter := range eligible {
			group.Go(func() {
				dialer := net.Dialer{Timeout: 1800 * time.Millisecond, LocalAddr: &net.TCPAddr{IP: adapter.IP}, Control: nics.ForceEgress(adapter.Index, false)}
				if conn, err := dialer.DialContext(ctx, "tcp4", net.JoinHostPort(host, "443")); err == nil {
					_ = conn.Close()
					ready[i] = true
				}
			})
		}
		group.Wait()
		plan := routingTarget{host: host, port: 443}
		selectedReachable := tetherAdapter == ""
		for i, reachable := range ready {
			if reachable {
				plan.links = append(plan.links, eligible[i])
				selectedReachable = selectedReachable || eligible[i].Name == tetherAdapter
			}
		}
		if selectedReachable && len(plan.links) >= 2 {
			return plan, nil
		}
	}
	if tetherAdapter != "" {
		return routingTarget{}, fmt.Errorf("could not reach a shared IPv4 probe through %q and another eligible adapter", tetherAdapter)
	}
	return routingTarget{}, fmt.Errorf("fewer than two eligible adapters reached the same IPv4 probe")
}

func diagnosticDial(ctx context.Context, balancer *engine.Balancer, plan routingTarget) (net.Conn, *engine.Slot, func(), error) {
	conn, slot, family, err := balancer.DialTCP(ctx, plan.host, plan.port)
	if err != nil {
		return nil, nil, nil, err
	}
	return conn, slot, func() {
		_ = conn.Close()
		balancer.S.ReleaseFor(slot, family)
	}, nil
}

func checkSpeed(plan routingTarget) (TestStatus, string) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	scheduler := engine.NewScheduler(plan.links)
	balancer := engine.NewBalancer(scheduler)
	balancer.ConnectTimeout = 2 * time.Second
	expected := make(map[string]net.IP, len(plan.links))
	for _, link := range plan.links {
		expected[link.Name] = link.IP
	}
	seen := make(map[string]bool)
	var names []string
	for range plan.links {
		conn, slot, closeFlow, err := diagnosticDial(ctx, balancer, plan)
		if err != nil {
			return StatusFail, "Speed flow failed: " + err.Error()
		}
		defer closeFlow() // Keep every flow open while the scheduler picks the next adapter.
		if !conn.LocalAddr().(*net.TCPAddr).IP.Equal(expected[slot.Name]) {
			return StatusFail, fmt.Sprintf("Speed flow selected %q but used an unexpected source address", slot.Name)
		}
		if !seen[slot.Name] {
			seen[slot.Name] = true
			names = append(names, slot.Name)
		}
	}
	if len(seen) != len(plan.links) {
		return StatusFail, fmt.Sprintf("Speed opened flows through %d of %d reachable adapters: %s", len(seen), len(plan.links), strings.Join(names, ", "))
	}
	return StatusPass, fmt.Sprintf("Kept %d TCP connections to %s open concurrently through %s", len(plan.links), net.JoinHostPort(plan.host, strconv.Itoa(plan.port)), strings.Join(names, ", "))
}

func checkLiveFailover(plan routingTarget, tetherAdapter string) (TestStatus, string) {
	primary, secondary := plan.links[0], plan.links[1]
	for _, link := range plan.links {
		if link.Name == tetherAdapter {
			secondary = link
			if primary.Name == link.Name {
				primary = plan.links[1]
			}
			break
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 9*time.Second)
	defer cancel()
	scheduler := engine.NewScheduler([]nics.Nic{primary, secondary})
	scheduler.SetMode(engine.ModeBackup, primary.Name, secondary.Name)
	balancer := engine.NewBalancer(scheduler)
	balancer.ConnectTimeout = 2 * time.Second

	assertLink := func(step string, link nics.Nic) error {
		conn, slot, closeFlow, err := diagnosticDial(ctx, balancer, plan)
		if err != nil {
			return fmt.Errorf("%s: %w", step, err)
		}
		defer closeFlow()
		if slot.Name != link.Name || !conn.LocalAddr().(*net.TCPAddr).IP.Equal(link.IP) {
			return fmt.Errorf("%s chose %q instead of %q", step, slot.Name, link.Name)
		}
		return nil
	}
	if err := assertLink("healthy primary", primary); err != nil {
		return StatusFail, err.Error()
	}
	// Inject a loss of the primary source address into the diagnostic scheduler.
	// The first dial then fails at the real OS bind, exercising the production retry.
	broken := primary
	broken.IP = net.IPv4(192, 0, 2, 254)
	scheduler.Update([]nics.Nic{broken, secondary})
	if err := assertLink("primary bind failed", secondary); err != nil {
		return StatusFail, err.Error()
	}
	observedFailure := false
	for _, slot := range scheduler.Snapshot() {
		if slot.Name == primary.Name && slot.Failed > 0 {
			observedFailure = true
			break
		}
	}
	if !observedFailure {
		return StatusFail, "Backup connected, but the primary failure was not observed"
	}
	scheduler.Update([]nics.Nic{primary, secondary})
	if err := assertLink("primary recovered", primary); err != nil {
		return StatusFail, err.Error()
	}
	return StatusPass, fmt.Sprintf("Real TCP flows used %q → %q → %q after an injected primary bind failure and recovery in the diagnostic engine", primary.Name, secondary.Name, primary.Name)
}
