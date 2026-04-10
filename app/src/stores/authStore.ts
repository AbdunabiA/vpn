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
  logout: () => Promise<void>;
  setLoading: (loading: boolean) => void;
}

// Concurrency guard for initialize() — StrictMode double-mounts and rapid
// re-renders can otherwise call /auth/guest twice and orphan a server
// session. The first call sets this true and subsequent calls early-return.
let initializing = false;

export const useAuthStore = create<AuthState>((set, get) => ({
  user: null,
  tokens: null,
  isAuthenticated: false,
  isLoading: false,

  // Called once on app startup — restores tokens from storage or auto-creates a guest session.
  // The guest call sends the device fingerprint (id + secret) so the server
  // can return the existing user_id for this device across reinstalls and
  // across linked accounts via share codes.
  //
  // Guarded against concurrent invocation by the module-level `initializing`
  // flag — StrictMode double-mounts and resume-from-background can otherwise
  // call /auth/guest twice and orphan a server session.
  initialize: () => {
    if (initializing) return;
    initializing = true;
    set({isLoading: true});
    (async () => {
      try {
        const stored = await AsyncStorage.getItem(TOKENS_KEY);
        if (stored) {
          try {
            const tokens = JSON.parse(stored) as AuthTokens;
            set({tokens, isAuthenticated: true, isLoading: false});
            return;
          } catch {
            await AsyncStorage.removeItem(TOKENS_KEY);
          }
        }

        const fingerprint = await getDeviceFingerprint();
        const {data} = await api.post<{data: AuthTokens}>('/auth/guest', fingerprint);
        const tokens = data.data;
        await AsyncStorage.setItem(TOKENS_KEY, JSON.stringify(tokens));
        set({tokens, isAuthenticated: true, isLoading: false});
      } catch {
        // Guest login failed (e.g. no network). The app stays unauthenticated
        // and will retry the next time initialize() is called.
        set({isLoading: false});
      } finally {
        initializing = false;
      }
    })();
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

  // Awaits the AsyncStorage removal so a racing initialize() (e.g. caused
  // by app resume) cannot read the now-stale tokens before they are cleared.
  logout: async () => {
    try {
      await AsyncStorage.removeItem(TOKENS_KEY);
    } catch {
      // Even if persistence fails, drop the in-memory tokens so the UI
      // immediately reflects the logged-out state.
    }
    set({user: null, tokens: null, isAuthenticated: false});
  },

  setLoading: (loading: boolean) => set({isLoading: loading}),
}));
