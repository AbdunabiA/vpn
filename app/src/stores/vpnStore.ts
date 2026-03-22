import {create} from 'zustand';
import * as vpnBridge from '../services/vpnBridge';
import type {ConnectionState, TunnelStatus, TrafficStats} from '../types/vpn';
import type {Server, ServerConfig} from '../types/api';

interface VpnState {
  connectionState: ConnectionState;
  currentServer: Server | null;
  serverConfig: ServerConfig | null;
  connectedAt: Date | null;
  bytesUp: number;
  bytesDown: number;
  speedUp: number;
  speedDown: number;
  error: string | null;

  connect: (server: Server, config: ServerConfig) => Promise<void>;
  disconnect: () => Promise<void>;
  updateStatus: (status: TunnelStatus) => void;
  updateStats: (stats: TrafficStats) => void;
  clearError: () => void;
}

export const useVpnStore = create<VpnState>((set, get) => ({
  connectionState: 'disconnected',
  currentServer: null,
  serverConfig: null,
  connectedAt: null,
  bytesUp: 0,
  bytesDown: 0,
  speedUp: 0,
  speedDown: 0,
  error: null,

  connect: async (server: Server, config: ServerConfig) => {
    const {connectionState} = get();
    if (connectionState === 'connected' || connectionState === 'connecting') {
      return;
    }

    set({
      connectionState: 'connecting',
      currentServer: server,
      serverConfig: config,
      error: null,
    });

    try {
      // Call native module via VPN bridge → Android VpnService / iOS NEVPNManager
      await vpnBridge.connect(config);

      set({
        connectionState: 'connected',
        connectedAt: new Date(),
      });
    } catch (err) {
      set({
        connectionState: 'error',
        error: err instanceof Error ? err.message : 'Connection failed',
      });
    }
  },

  disconnect: async () => {
    const {connectionState} = get();
    if (connectionState === 'disconnected') {
      return;
    }

    set({connectionState: 'disconnecting'});

    try {
      await vpnBridge.disconnect();

      set({
        connectionState: 'disconnected',
        connectedAt: null,
        bytesUp: 0,
        bytesDown: 0,
        speedUp: 0,
        speedDown: 0,
      });
    } catch (err) {
      set({
        connectionState: 'error',
        error: err instanceof Error ? err.message : 'Disconnect failed',
      });
    }
  },

  // Called by the native module event listener when tunnel status changes
  updateStatus: (status: TunnelStatus) => {
    set({
      connectionState: status.state,
      bytesUp: status.bytes_up,
      bytesDown: status.bytes_down,
      connectedAt: status.connected_at > 0 ? new Date(status.connected_at * 1000) : null,
      error: status.error || null,
    });
  },

  updateStats: (stats: TrafficStats) => {
    set({
      bytesUp: stats.bytes_up,
      bytesDown: stats.bytes_down,
      speedUp: stats.speed_up_bps,
      speedDown: stats.speed_down_bps,
    });
  },

  clearError: () => set({error: null}),
}));
