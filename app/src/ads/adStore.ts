import {create} from 'zustand';
import {MMKV} from 'react-native-mmkv';

const storage = new MMKV({id: 'ad-store'});

const CONNECT_COUNT_KEY = 'vpn-connect-count';

interface AdState {
  connectCount: number;
  interstitialReady: boolean;

  incrementConnectCount: () => number;
  resetConnectCount: () => void;
  setInterstitialReady: (ready: boolean) => void;
}

export const useAdStore = create<AdState>((set, get) => ({
  connectCount: storage.getNumber(CONNECT_COUNT_KEY) ?? 0,
  interstitialReady: false,

  incrementConnectCount: () => {
    const next = get().connectCount + 1;
    storage.set(CONNECT_COUNT_KEY, next);
    set({connectCount: next});
    return next;
  },

  resetConnectCount: () => {
    storage.set(CONNECT_COUNT_KEY, 0);
    set({connectCount: 0});
  },

  setInterstitialReady: (ready) => set({interstitialReady: ready}),
}));
