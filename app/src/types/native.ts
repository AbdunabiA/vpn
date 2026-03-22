// TurboModule interface for the VPN native module.
// This defines the contract between React Native JS and the
// native iOS/Android modules that wrap the Go tunnel library.

export interface VpnNativeModule {
  // Connect to a VPN server. configJSON is a serialized ConnectConfig.
  connect(configJSON: string): Promise<string>;

  // Disconnect from the current VPN server.
  disconnect(): Promise<string>;

  // Get current tunnel status as JSON string.
  getStatus(): Promise<string>;

  // Probe servers for latency. serversJSON is a JSON array of {address, port}.
  probeServers(serversJSON: string): Promise<string>;

  // Get current traffic stats as JSON string.
  getTrafficStats(): Promise<string>;
}

// Event names emitted by the native module
export const VPN_EVENTS = {
  STATUS_CHANGED: 'onVpnStatusChanged',
  STATS_UPDATED: 'onVpnStatsUpdated',
} as const;
