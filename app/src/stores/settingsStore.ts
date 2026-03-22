import {create} from 'zustand';
import type {VpnProtocol} from '../types/vpn';

type ThemeMode = 'dark' | 'light' | 'system';

interface SettingsState {
  protocol: VpnProtocol;
  killSwitch: boolean;
  splitTunneling: boolean;
  dnsOverHttps: boolean;
  language: 'en' | 'ru';
  theme: ThemeMode;

  setProtocol: (protocol: VpnProtocol) => void;
  setKillSwitch: (enabled: boolean) => void;
  setSplitTunneling: (enabled: boolean) => void;
  setDnsOverHttps: (enabled: boolean) => void;
  setLanguage: (lang: 'en' | 'ru') => void;
  setTheme: (theme: ThemeMode) => void;
}

export const useSettingsStore = create<SettingsState>((set) => ({
  protocol: 'auto',
  killSwitch: false,
  splitTunneling: false,
  dnsOverHttps: true,
  language: 'ru',
  theme: 'dark',

  setProtocol: (protocol) => set({protocol}),
  setKillSwitch: (killSwitch) => set({killSwitch}),
  setSplitTunneling: (splitTunneling) => set({splitTunneling}),
  setDnsOverHttps: (dnsOverHttps) => set({dnsOverHttps}),
  setLanguage: (language) => set({language}),
  setTheme: (theme) => set({theme}),
}));
