import {useQuery} from '@tanstack/react-query';
import {useEffect} from 'react';
import api from '../services/api';
import {useAuthStore} from '../stores/authStore';
import {useServerStore} from '../stores/serverStore';
import type {Server} from '../types/api';

async function fetchServers(): Promise<Server[]> {
  const {data} = await api.get<{data: Server[]}>('/servers');
  return data.data;
}

export function useServerList() {
  const isAuthenticated = useAuthStore(state => state.isAuthenticated);
  const setServers = useServerStore(state => state.setServers);

  const query = useQuery({
    queryKey: ['servers'],
    queryFn: fetchServers,
    staleTime: 30_000,
    refetchInterval: 60_000,
    enabled: isAuthenticated, // Don't fetch until logged in
  });

  // Sync fetched servers into the store so selectedServer is auto-populated
  useEffect(() => {
    if (query.data) {
      setServers(query.data);
    }
  }, [query.data, setServers]);

  return query;
}
