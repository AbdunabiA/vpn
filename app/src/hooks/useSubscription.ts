import {useQuery} from '@tanstack/react-query';
import {AppState, AppStateStatus} from 'react-native';
import {useEffect, useRef} from 'react';
import api from '../services/api';
import {useAuthStore} from '../stores/authStore';
import type {Subscription} from '../types/api';

async function fetchSubscription(): Promise<Subscription> {
  const {data} = await api.get<{data: Subscription}>('/subscription');
  return data.data;
}

// Hook for fetching the current user's subscription.
// Automatically refetches when the app returns to the foreground
// (e.g. when the user comes back from Stripe Checkout).
export function useSubscription() {
  const isAuthenticated = useAuthStore(state => state.isAuthenticated);

  const query = useQuery({
    queryKey: ['subscription'],
    queryFn: fetchSubscription,
    staleTime: 30_000,
    enabled: isAuthenticated,
  });

  // Refetch when app comes back to the foreground
  const appStateRef = useRef<AppStateStatus>(AppState.currentState);

  useEffect(() => {
    const subscription = AppState.addEventListener('change', (nextState) => {
      if (
        appStateRef.current !== 'active' &&
        nextState === 'active' &&
        isAuthenticated
      ) {
        query.refetch();
      }
      appStateRef.current = nextState;
    });

    return () => subscription.remove();
  }, [isAuthenticated, query]);

  return query;
}
