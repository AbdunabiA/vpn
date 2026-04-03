package tunnel

import "net"

// Russian IP CIDR supernets covering ~95% of RIPE NCC RU allocations.
// Used for client-side geo-detection as a fallback when the server
// does not provide protocol_priority hints.
var russianCIDRs []net.IPNet

func init() {
	cidrs := []string{
		"2.56.0.0/14",
		"5.0.0.0/12",
		"5.100.0.0/14",
		"5.128.0.0/11",
		"31.0.0.0/12",
		"31.128.0.0/11",
		"37.0.0.0/11",
		"37.140.0.0/14",
		"46.0.0.0/11",
		"46.146.0.0/15",
		"62.76.0.0/14",
		"77.32.0.0/11",
		"77.88.0.0/14",
		"78.24.0.0/13",
		"79.104.0.0/13",
		"80.64.0.0/13",
		"82.112.0.0/12",
		"83.136.0.0/12",
		"85.0.0.0/13",
		"85.192.0.0/11",
		"87.224.0.0/11",
		"89.104.0.0/13",
		"91.192.0.0/13",
		"93.80.0.0/12",
		"94.24.0.0/13",
		"95.24.0.0/13",
		"95.128.0.0/11",
		"109.184.0.0/13",
		"176.96.0.0/11",
		"178.0.0.0/12",
		"185.0.0.0/10",
		"188.128.0.0/11",
		"212.0.0.0/10",
		"217.64.0.0/11",
	}

	russianCIDRs = make([]net.IPNet, 0, len(cidrs))
	for _, cidr := range cidrs {
		_, ipNet, err := net.ParseCIDR(cidr)
		if err == nil {
			russianCIDRs = append(russianCIDRs, *ipNet)
		}
	}
}

// IsRussianIP checks whether the given IP address falls within known Russian
// IP ranges. This is a best-effort client-side check — the server-side GeoIP
// lookup (via MaxMind) is the authoritative source.
func IsRussianIP(ipStr string) bool {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}
	for _, cidr := range russianCIDRs {
		if cidr.Contains(ip) {
			return true
		}
	}
	return false
}

// DefaultProtocolPriority returns the recommended protocol order based on
// client IP geolocation. For Russian users, WebSocket (CDN) is preferred
// because TSPU cannot block Cloudflare without breaking major websites.
func DefaultProtocolPriority(clientIP string) []string {
	if IsRussianIP(clientIP) {
		return []string{"vless-ws", "amneziawg", "vless-reality"}
	}
	return []string{"vless-reality", "amneziawg", "vless-ws"}
}

// GetRecommendedProtocol returns the best protocol for the client's network
// conditions. It picks the first protocol from the geo-aware priority list
// that the server actually supports.
//
// clientIP: the client's public IP address
// serverProtocolsJSON: comma-separated list of protocols the server supports
// Returns: the recommended protocol string
func GetRecommendedProtocol(clientIP string, serverProtocols []string) string {
	priority := DefaultProtocolPriority(clientIP)
	supported := make(map[string]bool, len(serverProtocols))
	for _, p := range serverProtocols {
		supported[p] = true
	}
	for _, p := range priority {
		if supported[p] {
			return p
		}
	}
	// Fallback: return first available server protocol
	if len(serverProtocols) > 0 {
		return serverProtocols[0]
	}
	return "vless-reality"
}
