package nics

import (
	"net"
	"unsafe"

	"golang.org/x/sys/windows"
)

// GatewayIP returns the IPv4 default gateway used by one adapter, or nil when the
// adapter has no gateway. It uses the same GetAdaptersAddresses data as the
// discovery path so VPN/tunnel adapters are handled consistently.
func GatewayIP(idx int) net.IP {
	const flags = windows.GAA_FLAG_INCLUDE_GATEWAYS | windows.GAA_FLAG_SKIP_ANYCAST |
		windows.GAA_FLAG_SKIP_MULTICAST | windows.GAA_FLAG_SKIP_DNS_SERVER
	size := uint32(32 << 10)
	for try := 0; try < 4; try++ {
		buf := make([]byte, size)
		aa := (*windows.IpAdapterAddresses)(unsafe.Pointer(&buf[0]))
		err := windows.GetAdaptersAddresses(windows.AF_UNSPEC, flags, 0, aa, &size)
		if err == windows.ERROR_BUFFER_OVERFLOW {
			continue
		}
		if err != nil {
			return nil
		}
		for a := aa; a != nil; a = a.Next {
			if int(a.IfIndex) != idx && int(a.Ipv6IfIndex) != idx {
				continue
			}
			for g := a.FirstGatewayAddress; g != nil; g = g.Next {
				if g.Address.Sockaddr == nil || g.Address.Sockaddr.Addr.Family != windows.AF_INET {
					continue
				}
				sa := (*windows.SockaddrInet4)(unsafe.Pointer(g.Address.Sockaddr))
				ip := make(net.IP, 4)
				copy(ip, sa.Addr[:])
				return ip
			}
			return nil
		}
		return nil
	}
	return nil
}
