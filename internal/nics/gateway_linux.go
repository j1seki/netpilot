package nics

import (
	"net"

	"github.com/vishvananda/netlink"
)

// GatewayIP returns the IPv4 default gateway used by one adapter, or nil when the
// adapter has no default route. It is used for per-adapter "latency to router"
// probes without changing any routing state.
func GatewayIP(idx int) net.IP {
	routes, err := netlink.RouteListFiltered(netlink.FAMILY_V4, &netlink.Route{LinkIndex: idx}, netlink.RT_FILTER_OIF)
	if err == nil {
		for _, r := range routes {
			if r.Gw == nil {
				continue
			}
			if r.Dst == nil || (r.Dst.IP != nil && r.Dst.IP.IsUnspecified()) {
				return r.Gw
			}
		}
	}

	// Policy-routing setups sometimes keep the gateway in a per-interface table
	// rather than the main default route. RouteGet resolves the actual path.
	rts, err := netlink.RouteGet(net.IPv4(1, 1, 1, 1))
	if err != nil {
		return nil
	}
	for _, r := range rts {
		if r.LinkIndex == idx && r.Gw != nil {
			return r.Gw
		}
	}
	return nil
}
