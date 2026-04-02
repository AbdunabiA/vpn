import {create} from 'zustand';
import AsyncStorage from '@react-native-async-storage/async-storage';
import * as vpnBridge from '../services/vpnBridge';
import type {VpnProtocol} from '../types/vpn';

type ThemeMode = 'dark' | 'light' | 'system';

const SETTINGS_KEY = 'app-settings';

interface PersistedSettings {
  protocol: VpnProtocol;
  killSwitch: boolean;
  autoReconnect: boolean;
  excludedApps: string[];
  excludedDomains: string[];
}

interface SettingsState {
  protocol: VpnProtocol;
  killSwitch: boolean;
  splitTunneling: boolean;
  dnsOverHttps: boolean;
  language: 'en' | 'ru';
  theme: ThemeMode;
  autoReconnect: boolean;

  // Split tunneling exclusions:
  //   Android — package names, e.g. ["com.google.android.youtube"]
  //   iOS     — domain strings, e.g. ["banking.com", "work.internal"]
  excludedApps: string[];
  excludedDomains: string[];

  // Called once on app startup — restores persisted settings from storage
  initialize: () => void;

  setProtocol: (protocol: VpnProtocol) => void;
  setKillSwitch: (enabled: boolean) => void;
  setSplitTunneling: (enabled: boolean) => void;
  setDnsOverHttps: (enabled: boolean) => void;
  setLanguage: (lang: 'en' | 'ru') => void;
  setTheme: (theme: ThemeMode) => void;
  setAutoReconnect: (enabled: boolean) => void;
  setExcludedApps: (apps: string[]) => void;
  setExcludedDomains: (domains: string[]) => void;
}

/** Persists the subset of settings that survive app restarts. */
function persistSettings(partial: Partial<PersistedSettings>, current: SettingsState): void {
  const snapshot: PersistedSettings = {
    protocol: current.protocol,
    killSwitch: current.killSwitch,
    autoReconnect: current.autoReconnect,
    excludedApps: current.excludedApps,
    excludedDomains: current.excludedDomains,
    ...partial,
  };
  AsyncStorage.setItem(SETTINGS_KEY, JSON.stringify(snapshot)).catch((err) => {
    console.error('[Settings] persist failed:', err);
  });
}

export const useSettingsStore = create<SettingsState>((set, get) => ({
  protocol: 'auto',
  killSwitch: false,
  splitTunneling: false,
  dnsOverHttps: true,
  language: 'ru',
  theme: 'dark',
  autoReconnect: true,
  excludedApps: [],
  excludedDomains: [],

  initialize: () => {
    AsyncStorage.getItem(SETTINGS_KEY).then((stored) => {
      if (stored) {
        try {
          const saved = JSON.parse(stored) as Partial<PersistedSettings>;
          set({
            protocol: saved.protocol ?? 'auto',
            killSwitch: saved.killSwitch ?? false,
            autoReconnect: saved.autoReconnect ?? true,
            excludedApps: saved.excludedApps ?? [],
            excludedDomains: saved.excludedDomains ?? [],
          });
        } catch {
          AsyncStorage.removeItem(SETTINGS_KEY);
        }
      }
    });
  },

  setProtocol: (protocol) => {
    set({protocol});
    persistSettings({protocol}, get());
  },

  setKillSwitch: (killSwitch) => {
    set({killSwitch});
    persistSettings({killSwitch}, get());
    vpnBridge.setKillSwitch(killSwitch).catch((err) => {
      console.error('[Settings] setKillSwitch failed:', err);
    });
  },

  setSplitTunneling: (splitTunneling) => set({splitTunneling}),
  setDnsOverHttps: (dnsOverHttps) => set({dnsOverHttps}),
  setLanguage: (language) => set({language}),
  setTheme: (theme) => set({theme}),

  setAutoReconnect: (autoReconnect) => {
    set({autoReconnect});
    persistSettings({autoReconnect}, get());
  },

  setExcludedApps: (excludedApps) => {
    set({excludedApps});
    persistSettings({excludedApps}, get());
    vpnBridge.setExcludedApps(excludedApps).catch((err) => {
      console.error('[Settings] setExcludedApps failed:', err);
    });
  },

  setExcludedDomains: (excludedDomains) => {
    set({excludedDomains});
    persistSettings({excludedDomains}, get());
    vpnBridge.setExcludedDomains(excludedDomains).catch((err) => {
      console.error('[Settings] setExcludedDomains failed:', err);
    });
  },
}));
