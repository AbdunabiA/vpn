import React, {useEffect} from 'react';
import {StatusBar} from 'react-native';
import {SafeAreaProvider} from 'react-native-safe-area-context';
import {NavigationContainer} from '@react-navigation/native';
import {QueryClient, QueryClientProvider} from '@tanstack/react-query';
import {MobileAds} from 'yandex-mobile-ads';

import {RootNavigator} from './src/navigation/RootNavigator';
import {useAuthStore} from './src/stores/authStore';
import {ErrorBoundary} from './src/components/ErrorBoundary';
import {colors} from './src/theme';

// Import i18n to initialize translations
import './src/i18n';

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: 2,
      staleTime: 30_000,
    },
  },
});

const navTheme = {
  dark: true,
  colors: {
    primary: colors.primary,
    background: colors.background,
    card: colors.surface,
    text: colors.textPrimary,
    border: colors.border,
    notification: colors.error,
  },
  fonts: {
    regular: {fontFamily: 'System', fontWeight: '400' as const},
    medium: {fontFamily: 'System', fontWeight: '500' as const},
    bold: {fontFamily: 'System', fontWeight: '700' as const},
    heavy: {fontFamily: 'System', fontWeight: '900' as const},
  },
};

function App(): React.JSX.Element {
  // Restore auth tokens from MMKV on app launch
  useEffect(() => {
    useAuthStore.getState().initialize();

    // Initialize Yandex Ads SDK (fire-and-forget, failure is non-fatal)
    MobileAds.initialize().catch(err => {
      console.warn('[Ads] SDK initialization failed:', err);
    });
  }, []);

  return (
    <ErrorBoundary>
      <QueryClientProvider client={queryClient}>
        <SafeAreaProvider>
          <StatusBar barStyle="light-content" backgroundColor={colors.background} />
          <NavigationContainer theme={navTheme}>
            <RootNavigator />
          </NavigationContainer>
        </SafeAreaProvider>
      </QueryClientProvider>
    </ErrorBoundary>
  );
}

export default App;
