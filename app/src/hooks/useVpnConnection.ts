import {useEffect, useCallback, useRef} from 'react';
import NetInfo from '@react-native-community/netinfo';
import {useVpnStore} from '../stores/vpnStore';
import {useServerStore} from '../stores/serverStore';
import {useSettingsStore} from '../stores/settingsStore';
import * as vpnBridge from '../services/vpnBridge';
import api from '../services/api';
import type {ServerConfig} from '../types/api';

const MAX_RECONNECT_ATTEMPTS = 3;
const MAX_PROTOCOL_FALLBACKS = 3;
const BASE_RECONNECT_DELAY_MS = 1000;
const MAX_RECONNECT_DELAY_MS = 30_000;

function getBackoffDelay(attempt: number): number {
  const delay = BASE_RECONNECT_DELAY_MS * Math.pow(2, attempt);
  return Math.min(delay, MAX_RECONNECT_DELAY_MS);
}

/**
 * Builds an ordered list of protocols to try, based on server capabilities
 * and priority hints from the API (geo-aware).
 */
function buildProtocolQueue(
  config: ServerConfig,
  userProtocol: string,
): string[] {
  // Start with server-provided priority (geo-aware) or a default
  const priority = config.protocol_priority ?? [
    'vless-reality',
    'amneziawg',
    'vless-ws',
  ];

  // Filter to only include protocols the server actually supports
  const available = priority.filter(p => {
    if (p === 'vless-reality' && config.reality) return true;
    if (p === 'vless-ws' && config.websocket) return true;
    if (p === 'amneziawg' && config.awg) return true;
    return false;
  });

  // If user selected a specific protocol (not "auto"), move it to the front
  if (userProtocol !== 'auto' && available.includes(userProtocol)) {
    return [
      userProtocol,
      ...available.filter(p => p !== userProtocol),
    ];
  }

  // Fallback: if no protocols matched (misconfigured server), use the server's default
  if (available.length === 0) {
    return [config.protocol];
  }

  return available;
}

// Hook that manages the VPN connection lifecycle with protocol fallback.
export function useVpnConnection() {
  const {
    connectionState,
    currentServer,
    connectedAt,
    bytesUp,
    bytesDown,
    speedUp,
    speedDown,
    error,
    reconnectAttempt,
    connectionId,
    connect: storeConnect,
    disconnect: storeDisconnect,
    updateStatus,
    updateStats,
    clearError,
    setReconnectAttempt,
    setConnectionId,
  } = useVpnStore();

  const {selectedServer} = useServerStore();
  const {autoReconnect, protocol: userProtocol} = useSettingsStore();

  // Track the previous connection state to detect unexpected disconnects
  const prevStateRef = useRef(connectionState);
  // Track whether the disconnect was user-initiated
  const isManualDisconnectRef = useRef(false);
  // Reconnect timer handle
  const reconnectTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  // Protocol fallback state
  const fallbackRef = useRef({
    queue: [] as string[],
    index: 0,
    attemptPerProtocol: 0,
    config: null as ServerConfig | null,
  });

  // Listen for native status change events
  useEffect(() => {
    const unsubStatus = vpnBridge.onStatusChanged(updateStatus);
    const unsubStats = vpnBridge.onStatsUpdated(updateStats);

    return () => {
      unsubStatus();
      unsubStats();
    };
  }, [updateStatus, updateStats]);

  // Reserve a connection slot on the backend. Returns the connection ID or null on failure.
  const reserveConnection = useCallback(
    async (serverId: string): Promise<string | null> => {
      try {
        const res = await api.post<{data: {id: string}}>('/connections', {
          server_id: serverId,
        });
        const id = res.data.data.id;
        setConnectionId(id);
        return id;
      } catch (err) {
        console.error('[VPN Connection] Failed to reserve connection slot:', err);
        return null;
      }
    },
    [setConnectionId],
  );

  // Unregister connection from backend on disconnect
  const unregisterConnection = useCallback(
    async (id: string) => {
      try {
        await api.delete(`/connections/${id}`);
      } catch (err) {
        console.error('[VPN Connection] Failed to unregister connection:', err);
      } finally {
        setConnectionId(null);
      }
    },
    [setConnectionId],
  );

  // Try connecting with the next protocol in the fallback queue
  const tryNextProtocol = useCallback(async () => {
    const fb = fallbackRef.current;
    if (!fb.config || fb.index >= fb.queue.length || fb.index >= MAX_PROTOCOL_FALLBACKS) {
      // All protocols exhausted
      useVpnStore.setState({
        connectionState: 'error',
        error: 'All protocols blocked',
      });
      return;
    }

    const nextProtocol = fb.queue[fb.index];
    console.log(
      `[VPN Connection] Switching to protocol: ${nextProtocol} (${fb.index + 1}/${fb.queue.length})`,
    );

    useVpnStore.setState({connectionState: 'switching_protocol'});

    const server = selectedServer || currentServer;
    if (!server) return;

    try {
      // Ensure we have a connection slot reserved
      if (!useVpnStore.getState().connectionId) {
        const reserved = await reserveConnection(server.id);
        if (!reserved) {
          useVpnStore.setState({
            connectionState: 'error',
            error: 'Device limit reached',
          });
          return;
        }
      }

      // Re-fetch config in case server updated priority hints
      const {data} = await api.get<{data: ServerConfig}>(
        `/servers/${server.id}/config`,
      );
      fb.config = data.data;

      // Rebuild queue from fresh config (server may have changed priorities)
      const freshQueue = buildProtocolQueue(data.data, userProtocol);
      const currentProtocolInFresh = freshQueue.indexOf(nextProtocol);
      if (currentProtocolInFresh >= 0) {
        fb.queue = freshQueue;
        fb.index = currentProtocolInFresh;
      }

      // Override the protocol in the config for the Go tunnel
      const configWithProtocol = {
        ...data.data,
        protocol: nextProtocol,
      };

      await storeConnect(server, configWithProtocol);
      fb.attemptPerProtocol = 0;
    } catch (err) {
      console.error(
        `[VPN Connection] Failed with protocol ${nextProtocol}:`,
        err,
      );
      // Move to next protocol
      fb.index++;
      fb.attemptPerProtocol = 0;
      // Small delay before trying next protocol
      if (reconnectTimerRef.current) clearTimeout(reconnectTimerRef.current);
      reconnectTimerRef.current = setTimeout(() => tryNextProtocol(), 1000);
    }
  }, [selectedServer, currentServer, storeConnect, userProtocol, reserveConnection]);

  // Watch for state transitions to handle auto-reconnect
  useEffect(() => {
    const prevState = prevStateRef.current;

    // On transition to connected: reset fallback state (connection was already reserved before tunnel connect)
    if (prevState !== 'connected' && connectionState === 'connected') {
      fallbackRef.current.attemptPerProtocol = 0;
    }

    // On transition from connected/reconnecting to disconnected: unregister + maybe reconnect
    if (
      (prevState === 'connected' ||
        prevState === 'reconnecting' ||
        prevState === 'switching_protocol') &&
      connectionState === 'disconnected'
    ) {
      // Unregister the connection
      if (connectionId) {
        unregisterConnection(connectionId);
      }

      // Auto-reconnect with protocol fallback
      if (
        !isManualDisconnectRef.current &&
        autoReconnect &&
        (selectedServer || currentServer)
      ) {
        const fb = fallbackRef.current;
        fb.attemptPerProtocol++;

        if (fb.attemptPerProtocol >= MAX_RECONNECT_ATTEMPTS) {
          // Current protocol is failing — try next one
          fb.index++;
          fb.attemptPerProtocol = 0;

          if (fb.index < fb.queue.length && fb.index < MAX_PROTOCOL_FALLBACKS) {
            reconnectTimerRef.current = setTimeout(
              () => tryNextProtocol(),
              1000,
            );
          } else {
            useVpnStore.setState({
              connectionState: 'error',
              error: 'All protocols blocked',
            });
          }
        } else {
          // Retry same protocol with backoff
          const delay = getBackoffDelay(fb.attemptPerProtocol);
          setReconnectAttempt(fb.attemptPerProtocol);
          useVpnStore.setState({connectionState: 'reconnecting'});

          reconnectTimerRef.current = setTimeout(async () => {
            const server = selectedServer || currentServer;
            if (!server || !fb.config) return;
            try {
              // Ensure we have a connection slot reserved for the reconnect
              if (!useVpnStore.getState().connectionId) {
                const reserved = await reserveConnection(server.id);
                if (!reserved) {
                  useVpnStore.setState({
                    connectionState: 'error',
                    error: 'Device limit reached',
                  });
                  return;
                }
              }
              const configWithProtocol = {
                ...fb.config,
                protocol: fb.queue[fb.index] || fb.config.protocol,
              };
              await storeConnect(server, configWithProtocol);
            } catch (err) {
              console.error('[VPN Connection] Reconnect attempt failed:', err);
            }
          }, delay);
        }
      }
    }

    prevStateRef.current = connectionState;
  }, [
    connectionState,
    connectionId,
    autoReconnect,
    reconnectAttempt,
    currentServer,
    selectedServer,
    unregisterConnection,
    reserveConnection,
    setReconnectAttempt,
    storeConnect,
    tryNextProtocol,
  ]);

  // Cleanup reconnect timer on unmount
  useEffect(() => {
    return () => {
      if (reconnectTimerRef.current) {
        clearTimeout(reconnectTimerRef.current);
      }
    };
  }, []);

  // Send heartbeat every 60s while connected
  useEffect(() => {
    if (connectionState !== 'connected' || !connectionId) return;

    const interval = setInterval(async () => {
      try {
        await api.patch(`/connections/${connectionId}/heartbeat`);
      } catch (err) {
        console.error('[VPN Connection] Heartbeat failed:', err);
      }
    }, 60_000);

    // Send initial heartbeat immediately on connect
    api.patch(`/connections/${connectionId}/heartbeat`).catch(() => {});

    return () => clearInterval(interval);
  }, [connectionState, connectionId]);

  // Detect network recovery and trigger reconnection when VPN was active
  const wasConnectedRef = useRef(false);

  useEffect(() => {
    if (connectionState === 'connected') {
      wasConnectedRef.current = true;
    } else if (
      connectionState === 'disconnected' ||
      connectionState === 'error'
    ) {
      // Reset only on manual disconnect (auto-reconnect keeps it true)
      if (isManualDisconnectRef.current) {
        wasConnectedRef.current = false;
      }
    }
  }, [connectionState]);

  useEffect(() => {
    const unsubscribe = NetInfo.addEventListener(state => {
      const isConnected = state.isConnected && state.isInternetReachable !== false;
      const vpnState = useVpnStore.getState().connectionState;

      // Network came back while VPN is in error/disconnected state and wasn't manually disconnected
      if (
        isConnected &&
        wasConnectedRef.current &&
        !isManualDisconnectRef.current &&
        autoReconnect &&
        (vpnState === 'error' || vpnState === 'disconnected') &&
        (selectedServer || currentServer)
      ) {
        console.log('[VPN Connection] Network recovered — attempting reconnect');
        wasConnectedRef.current = false; // prevent duplicate triggers

        // Small delay to let the network stabilize
        if (reconnectTimerRef.current) clearTimeout(reconnectTimerRef.current);
        reconnectTimerRef.current = setTimeout(async () => {
          const server = selectedServer || currentServer;
          if (!server) return;

          try {
            const {data} = await api.get<{data: ServerConfig}>(
              `/servers/${server.id}/config`,
            );
            const config = data.data;
            const queue = buildProtocolQueue(config, userProtocol);
            fallbackRef.current = {queue, index: 0, attemptPerProtocol: 0, config};

            const reservedId = await reserveConnection(server.id);
            if (!reservedId) {
              useVpnStore.setState({connectionState: 'error', error: 'Device limit reached'});
              return;
            }

            useVpnStore.setState({connectionState: 'reconnecting'});
            setReconnectAttempt(1);

            const configWithProtocol = {...config, protocol: queue[0] || config.protocol};
            await storeConnect(server, configWithProtocol);
          } catch (err) {
            console.error('[VPN Connection] Network recovery reconnect failed:', err);
          }
        }, 2000);
      }
    });

    return () => unsubscribe();
  }, [autoReconnect, selectedServer, currentServer, userProtocol, reserveConnection, storeConnect, setReconnectAttempt]);

  const connect = useCallback(async () => {
    const server = selectedServer;
    if (!server) return;

    isManualDisconnectRef.current = false;

    try {
      // 1. Fetch server config
      const {data} = await api.get<{data: ServerConfig}>(
        `/servers/${server.id}/config`,
      );
      const config = data.data;

      // 2. Reserve connection slot BEFORE connecting tunnel
      const reservedConnectionId = await reserveConnection(server.id);
      if (!reservedConnectionId) {
        useVpnStore.setState({
          connectionState: 'error',
          error: 'Device limit reached',
        });
        return;
      }

      // 3. Build protocol queue and connect tunnel
      const queue = buildProtocolQueue(config, userProtocol);
      fallbackRef.current = {
        queue,
        index: 0,
        attemptPerProtocol: 0,
        config,
      };

      const primaryProtocol = queue[0] || config.protocol;
      const configWithProtocol = {
        ...config,
        protocol: primaryProtocol,
      };

      console.log(
        `[VPN Connection] Connecting with protocol: ${primaryProtocol}, queue: [${queue.join(', ')}]`,
      );

      try {
        await storeConnect(server, configWithProtocol);
      } catch (err) {
        // 4. Tunnel failed — release reservation
        if (reservedConnectionId) {
          try {
            await api.delete(`/connections/${reservedConnectionId}`);
          } catch {}
          setConnectionId(null);
        }
        throw err;
      }
    } catch (err) {
      console.error('Failed to connect:', err);
      useVpnStore.setState({
        connectionState: 'error',
        error: err instanceof Error ? err.message : 'Connection failed',
      });
    }
  }, [selectedServer, storeConnect, userProtocol, reserveConnection]);

  const disconnect = useCallback(async () => {
    // Mark as manual so the auto-reconnect logic is suppressed
    isManualDisconnectRef.current = true;

    // Cancel any pending reconnect
    if (reconnectTimerRef.current) {
      clearTimeout(reconnectTimerRef.current);
      reconnectTimerRef.current = null;
    }
    setReconnectAttempt(0);
    fallbackRef.current = {queue: [], index: 0, attemptPerProtocol: 0, config: null};

    // Unregister BEFORE store clears tunnel — read fresh ID from store
    const currentId = useVpnStore.getState().connectionId;
    if (currentId) {
      await unregisterConnection(currentId);
    }

    await storeDisconnect();
  }, [storeDisconnect, setReconnectAttempt, unregisterConnection]);

  // Toggle: connect if disconnected, disconnect if connected
  const toggle = useCallback(async () => {
    if (connectionState === 'connected') {
      await disconnect();
    } else if (
      connectionState === 'disconnected' ||
      connectionState === 'error'
    ) {
      await connect();
    }
  }, [connectionState, connect, disconnect]);

  return {
    connectionState,
    currentServer,
    connectedAt,
    bytesUp,
    bytesDown,
    speedUp,
    speedDown,
    error,
    reconnectAttempt,
    connect,
    disconnect,
    toggle,
    clearError,
  };
}
