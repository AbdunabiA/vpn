import {useEffect, useCallback} from 'react';
import {useVpnStore} from '../stores/vpnStore';
import {useServerStore} from '../stores/serverStore';
import * as vpnBridge from '../services/vpnBridge';
import api from '../services/api';
import type {ServerConfig} from '../types/api';

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
    connect: storeConnect,
    disconnect: storeDisconnect,
    updateStatus,
    updateStats,
    clearError,
  } = useVpnStore();

  const {selectedServer} = useServerStore();

  // Listen for native status change events
  useEffect(() => {
    const unsubStatus = vpnBridge.onStatusChanged(updateStatus);
    const unsubStats = vpnBridge.onStatsUpdated(updateStats);

    return () => {
      unsubStatus();
      unsubStats();
    };
  }, [updateStatus, updateStats]);

  const connect = useCallback(async () => {
    const server = selectedServer;
    if (!server) {
      return;
    }

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
    await storeDisconnect();
  }, [storeDisconnect]);

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
    connect,
    disconnect,
    toggle,
    clearError,
  };
}
