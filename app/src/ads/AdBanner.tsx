import React, {useState, useEffect} from 'react';
import {View, StyleSheet, useWindowDimensions} from 'react-native';
import {BannerView, BannerAdSize} from 'yandex-mobile-ads';
import {useAuthStore} from '../stores/authStore';
import {AD_UNIT_IDS} from './config';

export function AdBanner() {
  const tier = useAuthStore(s => s.user?.subscription_tier);
  const {width} = useWindowDimensions();
  const [adSize, setAdSize] = useState<BannerAdSize | null>(null);
  const [loaded, setLoaded] = useState(false);
  const [failed, setFailed] = useState(false);

  useEffect(() => {
    BannerAdSize.stickySize(width)
      .then(setAdSize)
      .catch(() => setFailed(true));
  }, [width]);

  // Hide for paying users (guest users default to free)
  if (tier && tier !== 'free') return null;
  if (!adSize || failed) return null;

  return (
    <View style={[styles.container, !loaded && styles.hidden]}>
      <BannerView
        size={adSize}
        adUnitId={AD_UNIT_IDS.banner}
        onAdLoaded={() => setLoaded(true)}
        onAdFailedToLoad={() => {
          setLoaded(false);
          setFailed(true);
        }}
      />
    </View>
  );
}

const styles = StyleSheet.create({
  container: {
    width: '100%',
    alignItems: 'center',
  },
  hidden: {
    height: 0,
    overflow: 'hidden',
  },
});
