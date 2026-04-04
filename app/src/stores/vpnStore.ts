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
  reconnectAttempt: number;
  connectionId: string | null;
  // True while connect() is in flight — prevents re-entrant calls
  _connecting: boolean;

  connect: (server: Server, config: ServerConfig) => Promise<void>;
  disconnect: () => Promise<void>;
  updateStatus: (status: TunnelStatus) => void;
  updateStats: (stats: TrafficStats) => void;
  clearError: () => void;
  setReconnectAttempt: (attempt: number) => void;
  setConnectionId: (id: string | null) => void;
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
  reconnectAttempt: 0,
  connectionId: null,
  _connecting: false,

  connect: async (server: Server, config: ServerConfig) => {
    const {connectionState, _connecting} = get();
    // Block if already connecting, connected, or a connect call is in flight
    if (_connecting || connectionState === 'connected' || connectionState === 'connecting') {
      console.log('[VPN Store] connect blocked: _connecting=', _connecting, 'state=', connectionState);
      return;
    }

    set({
      _connecting: true,
      connectionState: 'connecting',
      currentServer: server,
      serverConfig: config,
      error: null,
    });

    try {
      // Call native module via VPN bridge → Android VpnService / iOS NEVPNManager
      // The promise resolves AFTER TUN + tun2socks are fully set up (step10).
      await vpnBridge.connect(config);

      // Promise resolved = VPN fully connected (TUN + tun2socks ready).
      set({
        _connecting: false,
        connectionState: 'connected',
        connectedAt: new Date(),
        reconnectAttempt: 0,
      });
    } catch (err) {
      const errorMsg = err instanceof Error ? err.message : 'Connection failed';
      console.error('[VPN Store] connect error:', errorMsg);
      // Report error to API for debugging (fire-and-forget)
      try {
        const api = (await import('../services/api')).default;
        api.post('/debug/error', { error: errorMsg, action: 'connect' }).catch(() => {});
      } catch {}
      set({
        _connecting: false,
        connectionState: 'error',
        error: errorMsg,
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
        reconnectAttempt: 0,
        // connectionId is NOT cleared here — the hook's disconnect callback
        // reads it to send DELETE /connections/:id before clearing it.
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
    // Report errors to API for debugging
    if (status.state === 'error' && status.error) {
      import('../services/api').then(m => {
        m.default.post('/debug/error', {
          error: status.error,
          action: 'status_event',
          state: status.state,
        }).catch(() => {});
      }).catch(() => {});
    }

    // While connect() is in flight, do NOT overwrite connectionState from
    // native callbacks — the connect() promise flow owns the state.
    // Only update traffic stats and timing info.
    if (get()._connecting) {
      set({
        bytesUp: status.bytes_up,
        bytesDown: status.bytes_down,
        connectedAt: status.connected_at > 0 ? new Date(status.connected_at * 1000) : null,
      });
      return;
    }

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

  setReconnectAttempt: (attempt) => set({reconnectAttempt: attempt}),

  setConnectionId: (id) => set({connectionId: id}),
}));
