import {NativeModules, NativeEventEmitter, Platform} from 'react-native';
import type {TunnelStatus, TrafficStats} from '../types/vpn';
import type {ServerConfig} from '../types/api';
import {VPN_EVENTS} from '../types/native';

// The native module will be registered as "VpnModule" by the TurboModule.
// On iOS: bridged from Swift via VpnTurboModule.mm
// On Android: bridged from Kotlin via VpnTurboModule.kt
// Both delegate to the Go tunnel library compiled via gomobile.
const VpnModule = NativeModules.VpnModule;

let eventEmitter: NativeEventEmitter | null = null;

function getEmitter(): NativeEventEmitter {
  if (!eventEmitter) {
    eventEmitter = new NativeEventEmitter(VpnModule);
  }
  return eventEmitter;
}

// Connect to a VPN server using configuration from the API
export async function connect(config: ServerConfig): Promise<void> {
  if (!VpnModule) {
    // In development without native module, simulate
    if (__DEV__) {
      console.log('[VPN Bridge] Simulating connect:', config.server_address);
      return;
    }
    throw new Error('VPN native module not available');
  }

  const result = await VpnModule.connect(JSON.stringify(config));
  if (result) {
    throw new Error(result);
  }
}

// Disconnect from the current VPN server
export async function disconnect(): Promise<void> {
  if (!VpnModule) {
    if (__DEV__) {
      console.log('[VPN Bridge] Simulating disconnect');
      return;
    }
    throw new Error('VPN native module not available');
  }

  const result = await VpnModule.disconnect();
  if (result) {
    throw new Error(result);
  }
}

// Get current tunnel status
export async function getStatus(): Promise<TunnelStatus> {
  if (!VpnModule) {
    return {
      state: 'disconnected',
      server_addr: '',
      protocol: '',
      connected_at: 0,
      bytes_up: 0,
      bytes_down: 0,
    };
  }

  const json = await VpnModule.getStatus();
  return JSON.parse(json);
}

// Subscribe to tunnel status changes
export function onStatusChanged(
  callback: (status: TunnelStatus) => void,
): () => void {
  const emitter = getEmitter();
  const subscription = emitter.addListener(
    VPN_EVENTS.STATUS_CHANGED,
    (statusJSON: string) => {
      callback(JSON.parse(statusJSON));
    },
  );

  return () => subscription.remove();
}

// Subscribe to traffic stats updates
export function onStatsUpdated(
  callback: (stats: TrafficStats) => void,
): () => void {
  const emitter = getEmitter();
  const subscription = emitter.addListener(
    VPN_EVENTS.STATS_UPDATED,
    (statsJSON: string) => {
      callback(JSON.parse(statsJSON));
    },
  );

  return () => subscription.remove();
}

// Enable or disable the OS-level kill switch
export async function setKillSwitch(enabled: boolean): Promise<void> {
  if (!VpnModule) {
    if (__DEV__) {
      console.log('[VPN Bridge] Simulating setKillSwitch:', enabled);
      return;
    }
    throw new Error('VPN native module not available');
  }

  await VpnModule.setKillSwitch(enabled);
}

// Set apps that bypass the VPN (Android only — per-app split tunneling).
// apps: list of package name strings, e.g. ["com.google.android.youtube"].
export async function setExcludedApps(apps: string[]): Promise<void> {
  if (!VpnModule) {
    if (__DEV__) {
      console.log('[VPN Bridge] Simulating setExcludedApps:', apps);
      return;
    }
    throw new Error('VPN native module not available');
  }

  await VpnModule.setExcludedApps(JSON.stringify(apps));
}

export interface InstalledApp {
  packageName: string;
  appName: string;
  isSystemApp: boolean;
}

// Get installed apps that have INTERNET permission (Android only).
// On iOS returns an empty array — use setExcludedDomains instead.
export async function getInstalledApps(): Promise<InstalledApp[]> {
  if (!VpnModule) {
    if (__DEV__) {
      return [];
    }
    throw new Error('VPN native module not available');
  }

  const json = await VpnModule.getInstalledApps();
  return JSON.parse(json) as InstalledApp[];
}

// Set domains that bypass the VPN (iOS only — domain-based split tunneling).
// domains: list of domain strings, e.g. ["banking.com", "work.internal"].
export async function setExcludedDomains(domains: string[]): Promise<void> {
  if (!VpnModule) {
    if (__DEV__) {
      console.log('[VPN Bridge] Simulating setExcludedDomains:', domains);
      return;
    }
    throw new Error('VPN native module not available');
  }

  await VpnModule.setExcludedDomains(JSON.stringify(domains));
}

// Probe servers to find the fastest one
export async function probeServers(
  servers: Array<{address: string; port: number}>,
): Promise<Array<{server_address: string; latency_ms: number; success: boolean}>> {
  if (!VpnModule) {
    // Simulate probing in dev
    return servers.map((s) => ({
      server_address: `${s.address}:${s.port}`,
      latency_ms: Math.floor(Math.random() * 100) + 20,
      success: true,
    }));
  }

  const json = await VpnModule.probeServers(JSON.stringify(servers));
  return JSON.parse(json);
}
