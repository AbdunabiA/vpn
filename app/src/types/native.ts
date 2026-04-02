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

  // Enable or disable the OS-level kill switch.
  // On Android this sets an always-on VPN flag; on iOS it toggles NEOnDemand rules.
  setKillSwitch(enabled: boolean): Promise<void>;

  // Split tunneling — Android: per-app exclusion via VpnService.Builder.
  // appsJson is a JSON array of package name strings.
  setExcludedApps(appsJson: string): Promise<void>;

  // Query installed apps that declare INTERNET permission.
  // Returns a JSON array of {packageName, appName, isSystemApp}.
  // On iOS always returns "[]" (per-app VPN requires MDM).
  getInstalledApps(): Promise<string>;

  // Split tunneling — iOS: domain-based bypass via xray direct routing rule.
  // domainsJson is a JSON array of domain strings, e.g. ["banking.com"].
  setExcludedDomains(domainsJson: string): Promise<void>;
}

// Event names emitted by the native module
export const VPN_EVENTS = {
  STATUS_CHANGED: 'onVpnStatusChanged',
  STATS_UPDATED: 'onVpnStatsUpdated',
} as const;
