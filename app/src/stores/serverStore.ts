import {create} from 'zustand';
import type {Server} from '../types/api';

interface ServerState {
  servers: Server[];
  selectedServer: Server | null;
  favoriteIds: string[];
  isLoading: boolean;

  setServers: (servers: Server[]) => void;
  selectServer: (server: Server) => void;
  toggleFavorite: (serverId: string) => void;
  setLoading: (loading: boolean) => void;
}

export const useServerStore = create<ServerState>((set, get) => ({
  servers: [],
  selectedServer: null,
  favoriteIds: [],
  isLoading: false,

  setServers: (servers: Server[]) => set({servers}),

  selectServer: (server: Server) => set({selectedServer: server}),

  toggleFavorite: (serverId: string) => {
    const {favoriteIds} = get();
    const isFavorite = favoriteIds.includes(serverId);
    set({
      favoriteIds: isFavorite
        ? favoriteIds.filter(id => id !== serverId)
        : [...favoriteIds, serverId],
    });
  },

  setLoading: (loading: boolean) => set({isLoading: loading}),
}));
