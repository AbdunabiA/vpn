import {useQuery} from '@tanstack/react-query';
import api from '../services/api';
import {useAuthStore} from '../stores/authStore';
import type {Server} from '../types/api';

async function fetchServers(): Promise<Server[]> {
  const {data} = await api.get<{data: Server[]}>('/servers');
  return data.data;
}

export function useServerList() {
  const isAuthenticated = useAuthStore(state => state.isAuthenticated);

  return useQuery({
    queryKey: ['servers'],
    queryFn: fetchServers,
    staleTime: 30_000,
    refetchInterval: 60_000,
    enabled: isAuthenticated, // Don't fetch until logged in
  });
}
