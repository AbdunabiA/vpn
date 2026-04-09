import {useRef, useCallback, useEffect} from 'react';
import {
  InterstitialAd,
  InterstitialAdLoader,
  AdRequestConfiguration,
} from 'yandex-mobile-ads';
import {useAuthStore} from '../stores/authStore';
import {useAdStore} from './adStore';
import {
  AD_UNIT_IDS,
  INTERSTITIAL_EVERY_N_CONNECTS,
  INTERSTITIAL_TIMEOUT_MS,
} from './config';

/**
 * Manages interstitial ad lifecycle.
 * Returns `maybeShowInterstitial` — call before every VPN connect.
 * Resolves when the ad is dismissed, skipped, or times out.
 */
export function useInterstitialAd() {
  const adRef = useRef<InterstitialAd | null>(null);
  const loaderRef = useRef<InterstitialAdLoader | null>(null);

  const tier = useAuthStore(s => s.user?.subscription_tier);
  const isFree = !tier || tier === 'free';

  const preload = useCallback(async () => {
    if (!isFree) return;

    try {
      if (!loaderRef.current) {
        loaderRef.current = await InterstitialAdLoader.create();
      }

      const config = new AdRequestConfiguration(AD_UNIT_IDS.interstitial);
      const ad = await loaderRef.current.loadAd(config);
      adRef.current = ad;
      useAdStore.getState().setInterstitialReady(true);
    } catch (err) {
      console.warn('[Ads] Interstitial preload failed:', err);
      adRef.current = null;
      useAdStore.getState().setInterstitialReady(false);
    }
  }, [isFree]);

  // Preload on mount
  useEffect(() => {
    preload();
  }, [preload]);

  const maybeShowInterstitial = useCallback((): Promise<void> => {
    return new Promise(resolve => {
      // Non-free users: skip
      if (!isFree) {
        resolve();
        return;
      }

      // Increment counter and check if it's time
      const newCount = useAdStore.getState().incrementConnectCount();
      if (newCount % INTERSTITIAL_EVERY_N_CONNECTS !== 0) {
        resolve();
        return;
      }

      const ad = adRef.current;
      if (!ad) {
        // Ad not loaded — skip, preload for next time
        preload();
        resolve();
        return;
      }

      // Safety timeout — never block connect for more than N seconds
      let resolved = false;
      const timeout = setTimeout(() => {
        if (!resolved) {
          resolved = true;
          resolve();
        }
      }, INTERSTITIAL_TIMEOUT_MS);

      const finish = () => {
        if (!resolved) {
          resolved = true;
          clearTimeout(timeout);
          resolve();
        }
        // Preload next interstitial
        adRef.current = null;
        useAdStore.getState().setInterstitialReady(false);
        preload();
      };

      ad.onAdDismissed = finish;
      ad.onAdFailedToShow = finish;
      ad.onAdShown = () => {};
      ad.onAdClicked = () => {};
      ad.onAdImpression = () => {};

      try {
        ad.show();
      } catch {
        finish();
      }
    });
  }, [isFree, preload]);

  return {maybeShowInterstitial};
}
