// Ad configuration — Yandex Mobile Ads
// Test IDs auto-selected in __DEV__ builds

const TEST_BANNER_ID = 'demo-banner-yandex';
const TEST_INTERSTITIAL_ID = 'demo-interstitial-yandex';

// Real IDs from Yandex Partner Interface (https://partner.yandex.ru)
// App: Pulse Secure (ID 19086494) — created 2026-04-10
const PROD_BANNER_ID = 'R-M-19086494-1';       // Home Sticky Banner
const PROD_INTERSTITIAL_ID = 'R-M-19086494-2'; // Connect Interstitial

export const AD_UNIT_IDS = {
  banner: __DEV__ ? TEST_BANNER_ID : PROD_BANNER_ID,
  interstitial: __DEV__ ? TEST_INTERSTITIAL_ID : PROD_INTERSTITIAL_ID,
} as const;


// Show interstitial every N VPN connections (for free users only)
export const INTERSTITIAL_EVERY_N_CONNECTS = 3;

// Max time (ms) to wait for interstitial to dismiss before proceeding to connect
export const INTERSTITIAL_TIMEOUT_MS = 5000;
