package tunnel

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"sort"
	"sync"
	"time"
)

// ProbeResult holds the result of probing a single server with a specific protocol.
type ProbeResult struct {
	ServerAddress string `json:"server_address"`
	Protocol      string `json:"protocol"`
	Latency       int64  `json:"latency_ms"` // Round-trip time in milliseconds
	Success       bool   `json:"success"`
	Error         string `json:"error,omitempty"`
}

// ProbeServers tests connectivity to multiple servers in parallel
// and returns results sorted by latency (fastest first).
//
// This is called by the React Native app's protocol prober service
// to determine which protocol and server combination works best
// for the user's current network conditions.
//
// serversJSON: JSON array of server objects with "address", "port", and "protocol" fields
// Returns: JSON array of ProbeResult objects
func ProbeServers(serversJSON string) string {
	// Probes make direct connections that bypass the tunnel.
	// Block probing while connected to prevent real IP leaks.
	mgr := getManager()
	mgr.mu.Lock()
	state := mgr.status.State
	mgr.mu.Unlock()
	if state == StateConnected || state == StateConnecting {
		return `[{"error": "cannot probe while VPN is active — probes bypass the tunnel"}]`
	}

	type serverEntry struct {
		Address  string `json:"address"`
		Port     int    `json:"port"`
		Protocol string `json:"protocol"`
		WSHost   string `json:"ws_host,omitempty"`
		WSPath   string `json:"ws_path,omitempty"`
	}

	var servers []serverEntry
	if err := json.Unmarshal([]byte(serversJSON), &servers); err != nil {
		return fmt.Sprintf(`[{"error": "invalid input: %v"}]`, err)
	}

	var wg sync.WaitGroup
	results := make([]ProbeResult, len(servers))

	for i, srv := range servers {
		wg.Add(1)
		go func(idx int, s serverEntry) {
			defer wg.Done()
			protocol := s.Protocol
			if protocol == "" {
				protocol = "vless-reality"
			}
			switch protocol {
			case "vless-ws":
				results[idx] = probeWebSocket(s.WSHost)
			case "amneziawg":
				results[idx] = probeUDP(s.Address, s.Port)
			default:
				results[idx] = probeTCP(s.Address, s.Port)
			}
			results[idx].Protocol = protocol
		}(i, srv)
	}

	wg.Wait()

	// Sort by latency (successful probes first, then by latency)
	sortProbeResults(results)

	data, _ := json.Marshal(results)
	return string(data)
}

// probeTCP tests TCP connectivity to a single server.
// For REALITY servers, this is a TCP connection to port 443.
func probeTCP(address string, port int) ProbeResult {
	result := ProbeResult{
		ServerAddress: fmt.Sprintf("%s:%d", address, port),
	}

	start := time.Now()
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", address, port), 5*time.Second)
	if err != nil {
		result.Success = false
		result.Error = err.Error()
		return result
	}
	defer conn.Close()

	result.Latency = time.Since(start).Milliseconds()
	result.Success = true
	return result
}

// probeWebSocket tests HTTPS connectivity to a Cloudflare-proxied WebSocket host.
// A successful TLS handshake to the CDN domain means the CDN path is reachable.
// We only verify TLS connectivity, not the WebSocket upgrade — that requires
// a full HTTP request and is covered by the actual connection attempt.
func probeWebSocket(host string) ProbeResult {
	result := ProbeResult{
		ServerAddress: host,
	}

	if host == "" {
		result.Success = false
		result.Error = "no websocket host configured"
		return result
	}

	start := time.Now()
	conn, err := tls.DialWithDialer(
		&net.Dialer{Timeout: 5 * time.Second},
		"tcp",
		host+":443",
		&tls.Config{ServerName: host},
	)
	if err != nil {
		result.Success = false
		result.Error = err.Error()
		return result
	}
	defer conn.Close()

	result.Latency = time.Since(start).Milliseconds()
	result.Success = true
	return result
}

// probeUDP tests UDP connectivity for AmneziaWG.
// UDP is connectionless — we cannot verify the server is actually listening
// without a full WireGuard handshake. We send a probe packet and attempt to
// read a response. If no response arrives (expected for most UDP endpoints),
// we mark it as "probably reachable" with a penalty latency so TCP/TLS probes
// that got real responses sort first.
func probeUDP(address string, port int) ProbeResult {
	const penaltyLatencyMs = 5000 // sort below verified probes

	result := ProbeResult{
		ServerAddress: fmt.Sprintf("%s:%d", address, port),
	}

	conn, err := net.DialTimeout("udp", fmt.Sprintf("%s:%d", address, port), 5*time.Second)
	if err != nil {
		result.Success = false
		result.Error = err.Error()
		return result
	}
	defer conn.Close()

	// Send a minimal probe packet
	conn.SetDeadline(time.Now().Add(3 * time.Second))
	_, err = conn.Write([]byte{0x01, 0x00, 0x00, 0x00})
	if err != nil {
		result.Success = false
		result.Error = err.Error()
		return result
	}

	// Try to read a response — AmneziaWG won't respond to a malformed
	// handshake, but an ICMP port unreachable indicates the port is closed.
	buf := make([]byte, 128)
	_, readErr := conn.Read(buf)
	if readErr != nil {
		// No response — could mean the server is there (silently dropped) or not.
		// Mark as success with penalty so verified probes sort first.
		result.Success = true
		result.Latency = penaltyLatencyMs
		return result
	}

	// Got a response — server is definitely reachable.
	result.Success = true
	result.Latency = 1 // minimal — actual latency is in the sub-ms range for local calls
	return result
}

// sortProbeResults sorts results: successful first (by latency ascending), then failed.
func sortProbeResults(results []ProbeResult) {
	sort.Slice(results, func(i, j int) bool {
		if results[i].Success != results[j].Success {
			// Successful probes sort before failed ones.
			return results[i].Success
		}
		// Both have the same success status — sort by latency ascending.
		return results[i].Latency < results[j].Latency
	})
}
