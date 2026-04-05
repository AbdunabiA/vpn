import {create} from 'zustand';
import AsyncStorage from '@react-native-async-storage/async-storage';
import api from '../services/api';
import type {User, AuthTokens} from '../types/api';

const TOKENS_KEY = 'auth-tokens';

interface AuthState {
  user: User | null;
  tokens: AuthTokens | null;
  isAuthenticated: boolean;
  isLoading: boolean;

  // Actions
  initialize: () => void;
  login: (email: string, password: string) => Promise<void>;
  register: (email: string, password: string, name: string) => Promise<void>;
  fetchAccount: () => Promise<void>;
  updateProfile: (name: string) => Promise<void>;
  updateTokens: (tokens: AuthTokens) => void;
  logout: () => void;
  setLoading: (loading: boolean) => void;
}

export const useAuthStore = create<AuthState>((set, get) => ({
  user: null,
  tokens: null,
  isAuthenticated: false,
  isLoading: false,

  // Called once on app startup — restores tokens from storage or auto-creates a guest session
  initialize: () => {
    AsyncStorage.getItem(TOKENS_KEY).then(async stored => {
      if (stored) {
        try {
          const tokens = JSON.parse(stored) as AuthTokens;
          set({tokens, isAuthenticated: true});
          return;
        } catch {
          await AsyncStorage.removeItem(TOKENS_KEY);
        }
      }

      // No stored tokens — create a guest session automatically so the user
      // goes straight to the home screen without seeing a login screen.
      try {
        const {data} = await api.post<{data: AuthTokens}>('/auth/guest');
        const tokens = data.data;
        await AsyncStorage.setItem(TOKENS_KEY, JSON.stringify(tokens));
        set({tokens, isAuthenticated: true});
      } catch {
        // Guest login failed (e.g. no network). The app stays unauthenticated
        // and will retry the next time initialize() is called.
      }
    });
  },

  login: async (email: string, password: string) => {
    set({isLoading: true});
    try {
      const {data} = await api.post<{data: AuthTokens}>('/auth/login', {
        email,
        password,
      });
      const tokens = data.data;
      await AsyncStorage.setItem(TOKENS_KEY, JSON.stringify(tokens));
      set({tokens, isAuthenticated: true, isLoading: false});
    } catch (error) {
      set({isLoading: false});
      throw error;
    }
  },

  register: async (email: string, password: string, name: string) => {
    set({isLoading: true});
    try {
      const {data} = await api.post<{data: AuthTokens}>('/auth/register', {
        email,
        password,
        name,
      });
      const tokens = data.data;
      await AsyncStorage.setItem(TOKENS_KEY, JSON.stringify(tokens));
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

  updateProfile: async (name: string) => {
    const {data} = await api.patch<{data: {id: string; full_name: string}}>(
      '/account',
      {name},
    );
    set(state => ({
      user: state.user ? {...state.user, full_name: data.data.full_name} : null,
    }));
  },

  updateTokens: (tokens: AuthTokens) => {
    AsyncStorage.setItem(TOKENS_KEY, JSON.stringify(tokens));
    set({tokens});
  },

  logout: () => {
    AsyncStorage.removeItem(TOKENS_KEY);
    set({user: null, tokens: null, isAuthenticated: false});
  },

  setLoading: (loading: boolean) => set({isLoading: loading}),
}));
