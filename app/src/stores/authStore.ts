import {create} from 'zustand';
import AsyncStorage from '@react-native-async-storage/async-storage';
import api from '../services/api';
import {getDeviceFingerprint} from '../services/deviceFingerprint';
import type {User, AuthTokens} from '../types/api';

const TOKENS_KEY = 'auth-tokens';

interface AuthState {
  user: User | null;
  tokens: AuthTokens | null;
  isAuthenticated: boolean;
  isLoading: boolean;

  // Actions
  initialize: () => void;
  fetchAccount: () => Promise<void>;
  updateProfile: (name: string) => Promise<void>;
  updateTokens: (tokens: AuthTokens) => void;
  linkWithCode: (code: string) => Promise<void>;
  logout: () => void;
  setLoading: (loading: boolean) => void;
}

export const useAuthStore = create<AuthState>((set, get) => ({
  user: null,
  tokens: null,
  isAuthenticated: false,
  isLoading: false,

  // Called once on app startup — restores tokens from storage or auto-creates a guest session.
  // The guest call sends the device fingerprint so the server can return the
  // existing user_id for this device (across reinstalls and across linked
  // accounts via share codes).
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

      try {
        const fingerprint = await getDeviceFingerprint();
        const {data} = await api.post<{data: AuthTokens}>('/auth/guest', fingerprint);
        const tokens = data.data;
        await AsyncStorage.setItem(TOKENS_KEY, JSON.stringify(tokens));
        set({tokens, isAuthenticated: true});
      } catch {
        // Guest login failed (e.g. no network). The app stays unauthenticated
        // and will retry the next time initialize() is called.
      }
    });
  },

  // Redeem a 6-digit share code given by a friend (the plan owner). On
  // success, replaces the locally-stored tokens with ones for the owner's
  // user_id, so subsequent API calls behave as if this device had logged
  // into the owner's account directly.
  linkWithCode: async (code: string) => {
    set({isLoading: true});
    try {
      const fingerprint = await getDeviceFingerprint();
      const {data} = await api.post<{data: AuthTokens}>('/auth/link', {
        code,
        ...fingerprint,
      });
      const tokens = data.data;
      await AsyncStorage.setItem(TOKENS_KEY, JSON.stringify(tokens));
      set({tokens, isAuthenticated: true, isLoading: false, user: null});
      // Re-fetch the account so the UI immediately reflects the new tier.
      await get().fetchAccount();
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
