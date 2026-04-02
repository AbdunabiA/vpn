import {create} from 'zustand';
import * as vpnBridge from '../services/vpnBridge';
import type {VpnProtocol} from '../types/vpn';

type ThemeMode = 'dark' | 'light' | 'system';

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

export const useSettingsStore = create<SettingsState>((set) => ({
  protocol: 'auto',
  killSwitch: false,
  splitTunneling: false,
  dnsOverHttps: true,
  language: 'ru',
  theme: 'dark',
  autoReconnect: true,
  excludedApps: [],
  excludedDomains: [],

  setProtocol: (protocol) => set({protocol}),

  setKillSwitch: (killSwitch) => {
    set({killSwitch});
    vpnBridge.setKillSwitch(killSwitch).catch((err) => {
      console.error('[Settings] setKillSwitch failed:', err);
    });
  },

  setSplitTunneling: (splitTunneling) => set({splitTunneling}),
  setDnsOverHttps: (dnsOverHttps) => set({dnsOverHttps}),
  setLanguage: (language) => set({language}),
  setTheme: (theme) => set({theme}),
  setAutoReconnect: (autoReconnect) => set({autoReconnect}),

  setExcludedApps: (excludedApps) => {
    set({excludedApps});
    vpnBridge.setExcludedApps(excludedApps).catch((err) => {
      console.error('[Settings] setExcludedApps failed:', err);
    });
  },

  setExcludedDomains: (excludedDomains) => {
    set({excludedDomains});
    vpnBridge.setExcludedDomains(excludedDomains).catch((err) => {
      console.error('[Settings] setExcludedDomains failed:', err);
    });
  },
}));
