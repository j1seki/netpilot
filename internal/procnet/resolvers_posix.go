//go:build !windows

package procnet

import (
	"net/netip"
	"os"
	"strings"
)

// SystemResolvers reports the default resolver list, including any local DNS stub.
func SystemResolvers() []string {
	data, err := os.ReadFile("/etc/resolv.conf")
	if err != nil {
		return nil
	}
	var servers []string
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == "nameserver" {
			if ip, err := netip.ParseAddr(fields[1]); err == nil {
				servers = append(servers, ip.String())
			}
		}
	}
	return servers
}
