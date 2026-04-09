// Ad configuration — Yandex Mobile Ads
// Test IDs auto-selected in __DEV__ builds

const TEST_BANNER_ID = 'demo-banner-yandex';
const TEST_INTERSTITIAL_ID = 'demo-interstitial-yandex';

// Replace with real IDs from Yandex Partner Interface (https://partner.yandex.ru)
const PROD_BANNER_ID = 'R-M-XXXXXXX-Y';
const PROD_INTERSTITIAL_ID = 'R-M-XXXXXXX-Z';

export const AD_UNIT_IDS = {
  banner: __DEV__ ? TEST_BANNER_ID : PROD_BANNER_ID,
  interstitial: __DEV__ ? TEST_INTERSTITIAL_ID : PROD_INTERSTITIAL_ID,
} as const;

if (!__DEV__ && PROD_BANNER_ID.includes('XXXXXXX')) {
  console.error('[Ads] Production ad unit IDs are not configured! Register at https://partner.yandex.ru');
}

// Show interstitial every N VPN connections (for free users only)
export const INTERSTITIAL_EVERY_N_CONNECTS = 3;

// Max time (ms) to wait for interstitial to dismiss before proceeding to connect
export const INTERSTITIAL_TIMEOUT_MS = 5000;
