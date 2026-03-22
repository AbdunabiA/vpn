import {create} from 'zustand';
import {MMKV} from 'react-native-mmkv';
import api from '../services/api';
import type {User, AuthTokens} from '../types/api';

// Encrypted storage for auth tokens
const storage = new MMKV({id: 'auth-storage'});

interface AuthState {
  user: User | null;
  tokens: AuthTokens | null;
  isAuthenticated: boolean;
  isLoading: boolean;

  // Actions
  initialize: () => void;
  login: (email: string, password: string) => Promise<void>;
  register: (email: string, password: string) => Promise<void>;
  fetchAccount: () => Promise<void>;
  updateTokens: (tokens: AuthTokens) => void;
  logout: () => void;
  setLoading: (loading: boolean) => void;
}

export const useAuthStore = create<AuthState>((set, get) => ({
  user: null,
  tokens: null,
  isAuthenticated: false,
  isLoading: false,

  // Called once on app startup — restores tokens from MMKV
  initialize: () => {
    const stored = storage.getString('tokens');
    if (stored) {
      try {
        const tokens = JSON.parse(stored) as AuthTokens;
        set({tokens, isAuthenticated: true});
      } catch {
        storage.delete('tokens');
      }
    }
  },

  login: async (email: string, password: string) => {
    set({isLoading: true});
    try {
      const {data} = await api.post<{data: AuthTokens}>('/auth/login', {
        email,
        password,
      });
      const tokens = data.data;
      storage.set('tokens', JSON.stringify(tokens));
      set({tokens, isAuthenticated: true, isLoading: false});
    } catch (error) {
      set({isLoading: false});
      throw error;
    }
  },

  register: async (email: string, password: string) => {
    set({isLoading: true});
    try {
      const {data} = await api.post<{data: AuthTokens}>('/auth/register', {
        email,
        password,
      });
      const tokens = data.data;
      storage.set('tokens', JSON.stringify(tokens));
      set({tokens, isAuthenticated: true, isLoading: false});
    } catch (error) {
      set({isLoading: false});
      throw error;
    }
  },

  fetchAccount: async () => {
    try {
      const {data} = await api.get<{data: User}>('/account');
      set({user: data.data});
    } catch {
      // Silently fail — user info is not critical
    }
  },

  updateTokens: (tokens: AuthTokens) => {
    storage.set('tokens', JSON.stringify(tokens));
    set({tokens});
  },

  logout: () => {
    storage.delete('tokens');
    set({user: null, tokens: null, isAuthenticated: false});
  },

  setLoading: (loading: boolean) => set({isLoading: loading}),
}));
