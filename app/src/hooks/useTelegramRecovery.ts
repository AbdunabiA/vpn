import {useMutation, useQuery, useQueryClient} from '@tanstack/react-query';
import {Alert, Linking} from 'react-native';
import {useTranslation} from 'react-i18next';

import api from '../services/api';
import {useAuthStore} from '../stores/authStore';

// Shape of the /account/telegram-status response after envelope
// unwrapping. Matches the Go handler exactly — do not drift.
export interface TelegramStatus {
  linked: boolean;
  linked_at: string | null;
  // Cached from Telegram at link time — nullable because not every
  // user has a public @username, and pre-016 linked rows have both
  // fields as NULL until they unlink and relink.
  telegram_username: string | null;
  telegram_first_name: string | null;
}

// Shape of the /auth/telegram/{link,restore}-intent response. The
// backend returns an absolute Telegram deep link and a TTL in
// seconds so the UI can warn "ссылка истекает через 60 секунд" if
// the user dawdles.
interface IntentResponse {
  url: string;
  expires: number;
}

async function fetchTelegramStatus(): Promise<TelegramStatus> {
  const {data} = await api.get<{data: TelegramStatus}>(
    '/account/telegram-status',
  );
  return data.data;
}

async function requestLinkIntent(): Promise<IntentResponse> {
  const {data} = await api.post<{data: IntentResponse}>(
    '/auth/telegram/link-intent',
  );
  return data.data;
}

async function requestRestoreIntent(): Promise<IntentResponse> {
  const {data} = await api.post<{data: IntentResponse}>(
    '/auth/telegram/restore-intent',
  );
  return data.data;
}

async function unlinkTelegram(): Promise<void> {
  await api.delete('/account/telegram');
}

// useTelegramStatus returns the authenticated user's current
// Telegram binding status. Enabled only when the user is logged in.
export function useTelegramStatus() {
  const isAuthenticated = useAuthStore(state => state.isAuthenticated);
  return useQuery({
    queryKey: ['telegram-status'],
    queryFn: fetchTelegramStatus,
    enabled: isAuthenticated,
    staleTime: 30_000,
  });
}

// useTelegramLinkMutation opens a short-lived deep link in
// Telegram so the user can confirm the link from inside the bot.
// The mutation itself succeeds as soon as the URL is handed to
// Linking.openURL; the actual binding happens server-side when
// the bot receives /start link_<jwt>. The Account screen refetches
// telegram-status a few seconds later to confirm.
export function useTelegramLinkMutation() {
  const {t} = useTranslation();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async () => {
      const intent = await requestLinkIntent();
      const supported = await Linking.canOpenURL(intent.url);
      if (!supported) {
        throw new Error(t('telegram.openFailed'));
      }
      await Linking.openURL(intent.url);
    },
    onSuccess: async () => {
      // Invalidate so the UI refetches status when the user comes
      // back from Telegram. The bot's link handler persists the
      // binding before the user closes the bot chat.
      await qc.invalidateQueries({queryKey: ['telegram-status']});
    },
    onError: (err: unknown) => {
      const message =
        (err as {response?: {data?: {error?: string}}})?.response?.data?.error ??
        (err as Error)?.message ??
        t('telegram.linkFailed');
      Alert.alert(t('common.error'), message);
    },
  });
}

// useTelegramRestoreMutation is the twin of Link but for the
// restore flow — called from the fresh-install onboarding screen
// when the user remembers they have a linked Telegram account.
export function useTelegramRestoreMutation() {
  const {t} = useTranslation();
  return useMutation({
    mutationFn: async () => {
      const intent = await requestRestoreIntent();
      const supported = await Linking.canOpenURL(intent.url);
      if (!supported) {
        throw new Error(t('telegram.openFailed'));
      }
      await Linking.openURL(intent.url);
    },
    onError: (err: unknown) => {
      const message =
        (err as {response?: {data?: {error?: string}}})?.response?.data?.error ??
        (err as Error)?.message ??
        t('telegram.restoreFailed');
      Alert.alert(t('common.error'), message);
    },
  });
}

// useTelegramUnlinkMutation clears the binding so the user can
// link a new Telegram account from scratch. Idempotent server-side.
export function useTelegramUnlinkMutation() {
  const {t} = useTranslation();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: unlinkTelegram,
    onSuccess: async () => {
      await qc.invalidateQueries({queryKey: ['telegram-status']});
    },
    onError: (err: unknown) => {
      const message =
        (err as {response?: {data?: {error?: string}}})?.response?.data?.error ??
        (err as Error)?.message ??
        t('telegram.unlinkFailed');
      Alert.alert(t('common.error'), message);
    },
  });
}
