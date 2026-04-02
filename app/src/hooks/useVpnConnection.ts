import {useEffect, useCallback, useRef} from 'react';
import {useVpnStore} from '../stores/vpnStore';
import {useServerStore} from '../stores/serverStore';
import {useSettingsStore} from '../stores/settingsStore';
import * as vpnBridge from '../services/vpnBridge';
import api from '../services/api';
import type {ServerConfig} from '../types/api';

const MAX_RECONNECT_ATTEMPTS = 5;
const BASE_RECONNECT_DELAY_MS = 1000;
const MAX_RECONNECT_DELAY_MS = 30_000;

function getBackoffDelay(attempt: number): number {
  const delay = BASE_RECONNECT_DELAY_MS * Math.pow(2, attempt);
  return Math.min(delay, MAX_RECONNECT_DELAY_MS);
}

// Hook that manages the VPN connection lifecycle.
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
  const {autoReconnect} = useSettingsStore();

  // Track the previous connection state to detect unexpected disconnects
  const prevStateRef = useRef(connectionState);
  // Track whether the disconnect was user-initiated
  const isManualDisconnectRef = useRef(false);
  // Reconnect timer handle
  const reconnectTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  // Listen for native status change events
  useEffect(() => {
    const unsubStatus = vpnBridge.onStatusChanged(updateStatus);
    const unsubStats = vpnBridge.onStatsUpdated(updateStats);

    return () => {
      unsubStatus();
      unsubStats();
    };
  }, [updateStatus, updateStats]);

  // Register connection with backend after successful connect
  const registerConnection = useCallback(async () => {
    if (!currentServer) {
      return;
    }
    try {
      const {data} = await api.post<{data: {id: string}}>('/connections', {
        server_id: currentServer.id,
      });
      setConnectionId(data.data.id);
    } catch (err) {
      console.error('[VPN Connection] Failed to register connection:', err);
    }
  }, [currentServer, setConnectionId]);

  // Unregister connection from backend on disconnect
  const unregisterConnection = useCallback(async (id: string) => {
    try {
      await api.delete(`/connections/${id}`);
    } catch (err) {
      console.error('[VPN Connection] Failed to unregister connection:', err);
    } finally {
      setConnectionId(null);
    }
  }, [setConnectionId]);

  // Watch for state transitions to handle registration and auto-reconnect
  useEffect(() => {
    const prevState = prevStateRef.current;

    // On transition to connected: register device
    if (prevState !== 'connected' && connectionState === 'connected') {
      registerConnection();
    }

    // On transition from connected/reconnecting to disconnected: unregister + maybe reconnect
    if (
      (prevState === 'connected' || prevState === 'reconnecting') &&
      connectionState === 'disconnected'
    ) {
      // Unregister the connection
      if (connectionId) {
        unregisterConnection(connectionId);
      }

      // Auto-reconnect if not triggered manually and we have a server
      if (
        !isManualDisconnectRef.current &&
        autoReconnect &&
        (selectedServer || currentServer) &&
        reconnectAttempt < MAX_RECONNECT_ATTEMPTS
      ) {
        const nextAttempt = reconnectAttempt + 1;
        const delay = getBackoffDelay(reconnectAttempt);

        setReconnectAttempt(nextAttempt);
        useVpnStore.setState({connectionState: 'reconnecting'});

        reconnectTimerRef.current = setTimeout(async () => {
          const server = selectedServer || currentServer;
          if (!server) {
            return;
          }
          try {
            const {data} = await api.get<{data: ServerConfig}>(
              `/servers/${server.id}/config`,
            );
            await storeConnect(server, data.data);
          } catch (err) {
            console.error('[VPN Connection] Reconnect attempt failed:', err);
          }
        }, delay);
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
    registerConnection,
    unregisterConnection,
    setReconnectAttempt,
    storeConnect,
  ]);

  // Cleanup reconnect timer on unmount
  useEffect(() => {
    return () => {
      if (reconnectTimerRef.current) {
        clearTimeout(reconnectTimerRef.current);
      }
    };
  }, []);

  const connect = useCallback(async () => {
    const server = selectedServer;
    if (!server) {
      return;
    }

    isManualDisconnectRef.current = false;

    // Fetch real server config from API
    try {
      const {data} = await api.get<{data: ServerConfig}>(
        `/servers/${server.id}/config`,
      );
      await storeConnect(server, data.data);
    } catch (err) {
      console.error('Failed to fetch server config:', err);
    }
  }, [selectedServer, storeConnect]);

  const disconnect = useCallback(async () => {
    // Mark as manual so the auto-reconnect logic is suppressed
    isManualDisconnectRef.current = true;

    // Cancel any pending reconnect
    if (reconnectTimerRef.current) {
      clearTimeout(reconnectTimerRef.current);
      reconnectTimerRef.current = null;
    }
    setReconnectAttempt(0);

    await storeDisconnect();
  }, [storeDisconnect, setReconnectAttempt]);

  // Toggle: connect if disconnected, disconnect if connected
  const toggle = useCallback(async () => {
    if (connectionState === 'connected') {
      await disconnect();
    } else if (connectionState === 'disconnected' || connectionState === 'error') {
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
