package engine

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"strings"

	"netpilot/internal/nics"
)

// ParseDNSAddress accepts a DNS server IP, with an optional explicit port.
func ParseDNSAddress(value string) (netip.AddrPort, error) {
	server, err := netip.ParseAddrPort(value)
	if err != nil {
		if ip, parseErr := netip.ParseAddr(value); parseErr == nil {
			server = netip.AddrPortFrom(ip, 53)
		}
	}
	ip := server.Addr().Unmap()
	if !ip.IsValid() || ip.IsUnspecified() || ip.IsMulticast() || server.Port() == 0 {
		return netip.AddrPort{}, errors.New("custom DNS must be a unicast IPv4 or IPv6 address, optionally with a port")
	}
	return netip.AddrPortFrom(ip, server.Port()), nil
}

func (b *Balancer) dnsAddress(value string) (netip.AddrPort, error) {
	server, err := ParseDNSAddress(value)
	if err == nil && server.Addr().Is6() && !b.V6Allowed() {
		return netip.AddrPort{}, errors.New("custom DNS requires IPv6, which is unavailable under the current policy")
	}
	return server, err
}

func (b *Balancer) resolveHost(ctx context.Context, host string, policy FlowPolicy) ([]net.IP, error) {
	server, err := b.dnsAddress(policy.CustomDNS)
	if err != nil {
		return nil, err
	}
	f, family := Fam4, "4"
	if server.Addr().Is6() {
		f, family = Fam6, "6"
	}
	resolver := net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
			tried := map[string]struct{}{}
			lastErr := errors.New("no usable network adapter for custom DNS")
			for {
				sl := b.acquirePolicySlot(f, policy, tried)
				if sl == nil {
					return nil, lastErr
				}
				tried[sl.key] = struct{}{}
				d := net.Dialer{Control: nics.ForceEgress(sl.Index, f == Fam6), Timeout: b.ConnectTimeout}
				if strings.HasPrefix(network, "tcp") {
					network = "tcp"
					d.LocalAddr = &net.TCPAddr{IP: sl.ip(f)}
				} else {
					network = "udp"
					d.LocalAddr = &net.UDPAddr{IP: sl.ip(f)}
				}
				conn, err := d.DialContext(ctx, network+family, server.String())
				b.S.ReleaseFor(sl, f)
				if err == nil {
					return conn, nil
				}
				lastErr = err
				if ctx.Err() != nil {
					return nil, err
				}
			}
		},
	}
	network := "ip4"
	if b.V6Allowed() {
		network = "ip"
	}
	return resolver.LookupIP(ctx, network, strings.TrimSuffix(host, ".")+".")
}
