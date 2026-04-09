import {create} from 'zustand';
import AsyncStorage from '@react-native-async-storage/async-storage';

const CONNECT_COUNT_KEY = 'ad-connect-count';

// Load persisted count on startup (async — defaults to 0 until loaded)
let persistedCount = 0;
AsyncStorage.getItem(CONNECT_COUNT_KEY).then(val => {
  if (val !== null) {
    persistedCount = parseInt(val, 10) || 0;
    useAdStore.setState({connectCount: persistedCount});
  }
});

interface AdState {
  connectCount: number;
  interstitialReady: boolean;

  incrementConnectCount: () => number;
  resetConnectCount: () => void;
  setInterstitialReady: (ready: boolean) => void;
}

export const useAdStore = create<AdState>((set, get) => ({
  connectCount: persistedCount,
  interstitialReady: false,

  incrementConnectCount: () => {
    const next = get().connectCount + 1;
    AsyncStorage.setItem(CONNECT_COUNT_KEY, String(next)).catch(() => {});
    set({connectCount: next});
    return next;
  },

  resetConnectCount: () => {
    AsyncStorage.setItem(CONNECT_COUNT_KEY, '0').catch(() => {});
    set({connectCount: 0});
  },

  setInterstitialReady: (ready) => set({interstitialReady: ready}),
}));
