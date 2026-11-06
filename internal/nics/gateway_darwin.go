package nics

import "net"

// GatewayIP returns the IPv4 default gateway used by one adapter, or nil when the
// adapter has no default route. Scoped routes (macOS per-service defaults) count
// too, which is what makes a USB tether show a router even when it is not the
// primary service.
func GatewayIP(idx int) net.IP {
	routes, err := DefaultRoutes()
	if err != nil {
		return nil
	}
	for _, r := range routes {
		if r.Index == idx && !r.V6 && r.GW != nil {
			return r.GW
		}
	}
	for _, r := range routes {
		if r.Index == idx && r.GW != nil {
			return r.GW
		}
	}
	return nil
}
