// VPN connection states — mirrors the Go client-tunnel constants
export type ConnectionState =
  | 'disconnected'
  | 'connecting'
  | 'connected'
  | 'disconnecting'
  | 'reconnecting'
  | 'error';

// Tunnel status received from the native module (Go via gomobile)
export interface TunnelStatus {
  state: ConnectionState;
  server_addr: string;
  protocol: string;
  connected_at: number; // Unix timestamp, 0 if not connected
  bytes_up: number;
  bytes_down: number;
  error?: string;
}

// Traffic statistics from the Go tunnel
export interface TrafficStats {
  bytes_up: number;
  bytes_down: number;
  speed_up_bps: number;
  speed_down_bps: number;
  duration_secs: number;
}

// Protocol options available to the user
export type VpnProtocol = 'auto' | 'vless-reality' | 'amneziawg' | 'websocket';

// Server probe result from the Go protocol selector
export interface ProbeResult {
  server_address: string;
  protocol: string;
  latency_ms: number;
  success: boolean;
  error?: string;
}
