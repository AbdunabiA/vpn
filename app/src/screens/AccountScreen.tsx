import React, {useState, useEffect} from 'react';
import {
  View,
  Text,
  StyleSheet,
  SafeAreaView,
  ScrollView,
  TouchableOpacity,
  TextInput,
  ActivityIndicator,
  Alert,
  Share,
} from 'react-native';
import {useTranslation} from 'react-i18next';
import {useNavigation} from '@react-navigation/native';
import type {NativeStackNavigationProp} from '@react-navigation/native-stack';
import {useAuthStore} from '../stores/authStore';
import {useLayout} from '../hooks/useLayout';
import api from '../services/api';
import {colors, typography, spacing, borderRadius} from '../theme';
import type {RootStackParamList} from '../navigation/RootNavigator';

type NavigationProp = NativeStackNavigationProp<RootStackParamList>;

// Returns up to 2 initials from a display name.
function getInitials(name: string): string {
  const parts = name.trim().split(/\s+/).filter(Boolean);
  if (parts.length === 0) {
    return '?';
  }
  if (parts.length === 1) {
    return parts[0].charAt(0).toUpperCase();
  }
  return (parts[0].charAt(0) + parts[parts.length - 1].charAt(0)).toUpperCase();
}

function formatDate(iso: string): string {
  try {
    return new Date(iso).toLocaleDateString(undefined, {
      year: 'numeric',
      month: 'long',
      day: 'numeric',
    });
  } catch {
    return iso;
  }
}

function planDisplayName(tier: string, t: (key: string) => string): string {
  if (tier === 'premium') {
    return t('account.premiumPlan');
  }
  if (tier === 'ultimate') {
    return 'Ultimate';
  }
  return t('account.freePlan');
}

export function AccountScreen() {
  const {t} = useTranslation();
  const navigation = useNavigation<NavigationProp>();
  const {user, isAuthenticated, fetchAccount, updateProfile, linkWithCode, isLoading} =
    useAuthStore();
  const {tabletContentStyle, scale} = useLayout();

  const [isEditingName, setIsEditingName] = useState(false);
  const [nameInput, setNameInput] = useState('');
  const [nameSaving, setNameSaving] = useState(false);
  const [nameError, setNameError] = useState<string | null>(null);
  const [activeDevices, setActiveDevices] = useState<number | null>(null);

  // Plan sharing UI state
  const [shareCode, setShareCode] = useState<string | null>(null);
  const [shareLoading, setShareLoading] = useState(false);
  const [shareError, setShareError] = useState<string | null>(null);
  const [linkCodeInput, setLinkCodeInput] = useState('');
  const [linkMode, setLinkMode] = useState(false);
  const [linkError, setLinkError] = useState<string | null>(null);

  // Fetch account info and active connections on mount
  useEffect(() => {
    if (isAuthenticated) {
      fetchAccount();
      fetchActiveDevices();
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [isAuthenticated]);

  async function fetchActiveDevices() {
    try {
      const {data} = await api.get<{data: unknown[]}>('/connections');
      setActiveDevices(data.data.length);
    } catch {
      setActiveDevices(0);
    }
  }

  function startEditingName() {
    setNameInput(user?.full_name ?? '');
    setNameError(null);
    setIsEditingName(true);
  }

  function cancelEditingName() {
    setIsEditingName(false);
    setNameError(null);
  }

  async function saveName() {
    const trimmed = nameInput.trim();
    if (trimmed.length < 2) {
      setNameError(t('register.nameRequired'));
      return;
    }
    setNameSaving(true);
    setNameError(null);
    try {
      await updateProfile(trimmed);
      setIsEditingName(false);
    } catch {
      setNameError('Failed to save. Please try again.');
    } finally {
      setNameSaving(false);
    }
  }

  function handleLogout() {
    Alert.alert(t('account.logout'), '', [
      {text: t('common.cancel'), style: 'cancel'},
      {
        text: t('account.logout'),
        style: 'destructive',
        onPress: () => useAuthStore.getState().logout(),
      },
    ]);
  }

  // ---- Plan sharing handlers ----

  async function handleGenerateShareCode() {
    setShareLoading(true);
    setShareError(null);
    try {
      const {data} = await api.post<{data: {code: string; expires_in_sec: number}}>(
        '/devices/share-code',
      );
      setShareCode(data.data.code);
    } catch (err: unknown) {
      const anyErr = err as {response?: {data?: {error?: string}}};
      setShareError(anyErr?.response?.data?.error || t('share.errorGeneric'));
    } finally {
      setShareLoading(false);
    }
  }

  async function handleShareCodeViaSystem() {
    if (!shareCode) return;
    try {
      await Share.share({
        message: t('share.shareMessage', {code: shareCode}),
      });
    } catch {
      // User dismissed share sheet — no-op.
    }
  }

  async function handleSubmitLinkCode() {
    const trimmed = linkCodeInput.trim();
    if (trimmed.length !== 6 || !/^\d{6}$/.test(trimmed)) {
      setLinkError(t('share.codeFormatError'));
      return;
    }
    setLinkError(null);
    try {
      await linkWithCode(trimmed);
      setLinkMode(false);
      setLinkCodeInput('');
      Alert.alert(t('share.linkSuccessTitle'), t('share.linkSuccessBody'));
    } catch (err: unknown) {
      const anyErr = err as {response?: {data?: {error?: string}}};
      setLinkError(anyErr?.response?.data?.error || t('share.linkErrorGeneric'));
    }
  }

  if (!isAuthenticated) {
    // Guest auth runs automatically on app start; this branch is only hit
    // briefly during initialization or after a manual logout.
    return (
      <SafeAreaView style={styles.container}>
        <View style={styles.loadingContainer}>
          <ActivityIndicator size="large" color={colors.primary} />
        </View>
      </SafeAreaView>
    );
  }

  const displayName = user?.full_name || '?';
  const tier = user?.subscription_tier ?? 'free';
  const createdAt = user?.created_at ? formatDate(user.created_at) : '—';

  return (
    <SafeAreaView style={styles.container}>
      <ScrollView
        contentContainerStyle={[styles.content, tabletContentStyle]}
        showsVerticalScrollIndicator={false}>
        {/* Avatar + Name header */}
        <View style={styles.headerCard}>
          <View
            style={[
              styles.avatarCircle,
              {width: 72 * scale, height: 72 * scale, borderRadius: 36 * scale},
            ]}>
            <Text style={styles.avatarText}>{getInitials(displayName)}</Text>
          </View>

          {isEditingName ? (
            <View style={styles.nameEditRow}>
              <TextInput
                style={[styles.nameInput, nameError ? styles.nameInputError : null]}
                value={nameInput}
                onChangeText={setNameInput}
                placeholder={t('register.namePlaceholder')}
                placeholderTextColor={colors.textMuted}
                autoFocus
                maxLength={255}
                editable={!nameSaving}
              />
              {nameError && <Text style={styles.fieldError}>{nameError}</Text>}
              <View style={styles.nameEditActions}>
                <TouchableOpacity
                  style={styles.cancelNameButton}
                  onPress={cancelEditingName}
                  disabled={nameSaving}>
                  <Text style={styles.cancelNameText}>{t('common.cancel')}</Text>
                </TouchableOpacity>
                <TouchableOpacity
                  style={[styles.saveNameButton, nameSaving && styles.saveNameButtonDisabled]}
                  onPress={saveName}
                  disabled={nameSaving}>
                  {nameSaving ? (
                    <ActivityIndicator color={colors.textPrimary} size="small" />
                  ) : (
                    <Text style={styles.saveNameText}>{t('account.saveName')}</Text>
                  )}
                </TouchableOpacity>
              </View>
            </View>
          ) : (
            <TouchableOpacity
              style={styles.nameRow}
              onPress={startEditingName}
              accessibilityRole="button"
              accessibilityLabel={t('account.editName')}>
              <Text style={styles.displayName}>{displayName}</Text>
              <Text style={styles.editHint}>{t('account.editName')}</Text>
            </TouchableOpacity>
          )}
        </View>

        {/* Subscription card */}
        <View style={styles.card}>
          <Text style={styles.cardLabel}>{t('account.subscription')}</Text>
          <Text style={styles.cardValue}>{planDisplayName(tier, t)}</Text>
          {tier === 'free' && (
            <TouchableOpacity
              style={styles.upgradeButton}
              onPress={() => navigation.navigate('Payment')}
              activeOpacity={0.8}
              accessibilityRole="button"
              accessibilityLabel={t('account.upgradeToPremium')}>
              <Text style={styles.upgradeText}>{t('account.upgradeToPremium')}</Text>
            </TouchableOpacity>
          )}
        </View>

        {/* Plan sharing — only meaningful for paid tiers */}
        {tier !== 'free' && (
          <View style={styles.card}>
            <Text style={styles.cardLabel}>{t('share.shareTitle')}</Text>
            <Text style={styles.cardSubtle}>{t('share.shareSubtitle')}</Text>

            {shareCode ? (
              <View style={styles.codeBox}>
                <Text style={styles.codeValue}>{shareCode}</Text>
                <Text style={styles.codeHint}>{t('share.expiresHint')}</Text>
                <View style={styles.codeActionsRow}>
                  <TouchableOpacity
                    style={styles.codeActionBtn}
                    onPress={handleShareCodeViaSystem}>
                    <Text style={styles.codeActionText}>{t('share.shareButton')}</Text>
                  </TouchableOpacity>
                  <TouchableOpacity
                    style={[styles.codeActionBtn, styles.codeActionBtnSecondary]}
                    onPress={() => setShareCode(null)}>
                    <Text style={styles.codeActionTextSecondary}>{t('common.cancel')}</Text>
                  </TouchableOpacity>
                </View>
              </View>
            ) : (
              <TouchableOpacity
                style={styles.upgradeButton}
                onPress={handleGenerateShareCode}
                disabled={shareLoading}>
                {shareLoading ? (
                  <ActivityIndicator color={colors.textPrimary} />
                ) : (
                  <Text style={styles.upgradeText}>{t('share.generateButton')}</Text>
                )}
              </TouchableOpacity>
            )}
            {shareError && <Text style={styles.fieldError}>{shareError}</Text>}
          </View>
        )}

        {/* Link to existing plan — useful for free users who got a code from a friend */}
        {tier === 'free' && (
          <View style={styles.card}>
            <Text style={styles.cardLabel}>{t('share.linkTitle')}</Text>
            <Text style={styles.cardSubtle}>{t('share.linkSubtitle')}</Text>

            {linkMode ? (
              <View style={styles.linkInputBox}>
                <TextInput
                  style={styles.codeInput}
                  value={linkCodeInput}
                  onChangeText={setLinkCodeInput}
                  placeholder="000000"
                  placeholderTextColor={colors.textMuted}
                  keyboardType="number-pad"
                  maxLength={6}
                  autoFocus
                  editable={!isLoading}
                />
                {linkError && <Text style={styles.fieldError}>{linkError}</Text>}
                <View style={styles.codeActionsRow}>
                  <TouchableOpacity
                    style={[styles.codeActionBtn, styles.codeActionBtnSecondary]}
                    onPress={() => {
                      setLinkMode(false);
                      setLinkCodeInput('');
                      setLinkError(null);
                    }}
                    disabled={isLoading}>
                    <Text style={styles.codeActionTextSecondary}>{t('common.cancel')}</Text>
                  </TouchableOpacity>
                  <TouchableOpacity
                    style={styles.codeActionBtn}
                    onPress={handleSubmitLinkCode}
                    disabled={isLoading}>
                    {isLoading ? (
                      <ActivityIndicator color={colors.textPrimary} />
                    ) : (
                      <Text style={styles.codeActionText}>{t('share.linkButton')}</Text>
                    )}
                  </TouchableOpacity>
                </View>
              </View>
            ) : (
              <TouchableOpacity
                style={styles.upgradeButton}
                onPress={() => setLinkMode(true)}>
                <Text style={styles.upgradeText}>{t('share.openLinkButton')}</Text>
              </TouchableOpacity>
            )}
          </View>
        )}

        {/* Stats row */}
        <View style={styles.statsRow}>
          <View style={[styles.statCard, styles.statCardLeft]}>
            <Text style={styles.statLabel}>{t('account.memberSince')}</Text>
            <Text style={styles.statValue}>{createdAt}</Text>
          </View>
          <View style={[styles.statCard, styles.statCardRight]}>
            <Text style={styles.statLabel}>{t('account.activeDevices')}</Text>
            <Text style={styles.statValue}>
              {activeDevices === null ? '—' : String(activeDevices)}
            </Text>
          </View>
        </View>

        {/* Logout */}
        <TouchableOpacity
          style={styles.logoutButton}
          onPress={handleLogout}
          accessibilityRole="button">
          <Text style={styles.logoutText}>{t('account.logout')}</Text>
        </TouchableOpacity>
      </ScrollView>
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  container: {
    flex: 1,
    backgroundColor: colors.background,
  },
  content: {
    padding: spacing.md,
    paddingBottom: spacing.xxl,
  },
  loadingContainer: {
    flex: 1,
    alignItems: 'center',
    justifyContent: 'center',
  },

  // Header / avatar card
  headerCard: {
    backgroundColor: colors.surface,
    borderRadius: borderRadius.lg,
    padding: spacing.lg,
    borderWidth: 1,
    borderColor: colors.border,
    alignItems: 'center',
    marginBottom: spacing.md,
  },
  avatarCircle: {
    width: 72,
    height: 72,
    borderRadius: borderRadius.full,
    backgroundColor: colors.primaryDark,
    alignItems: 'center',
    justifyContent: 'center',
    marginBottom: spacing.md,
  },
  avatarText: {
    ...typography.h2,
    color: colors.textPrimary,
  },
  nameRow: {
    alignItems: 'center',
  },
  displayName: {
    ...typography.h3,
    color: colors.textPrimary,
    textAlign: 'center',
  },
  editHint: {
    ...typography.caption,
    color: colors.primary,
    marginTop: spacing.xs,
  },

  // Inline name editor
  nameEditRow: {
    width: '100%',
  },
  nameInput: {
    backgroundColor: colors.surfaceLight,
    borderRadius: borderRadius.sm,
    padding: spacing.md,
    color: colors.textPrimary,
    ...typography.body,
    borderWidth: 1,
    borderColor: colors.border,
    textAlign: 'center',
  },
  nameInputError: {
    borderColor: colors.error,
  },
  fieldError: {
    ...typography.caption,
    color: colors.error,
    textAlign: 'center',
    marginTop: spacing.xs,
  },
  nameEditActions: {
    flexDirection: 'row',
    marginTop: spacing.sm,
    gap: spacing.sm,
  },
  cancelNameButton: {
    flex: 1,
    paddingVertical: spacing.sm,
    borderRadius: borderRadius.sm,
    borderWidth: 1,
    borderColor: colors.border,
    alignItems: 'center',
  },
  cancelNameText: {
    ...typography.bodyBold,
    color: colors.textSecondary,
  },
  saveNameButton: {
    flex: 1,
    paddingVertical: spacing.sm,
    borderRadius: borderRadius.sm,
    backgroundColor: colors.primary,
    alignItems: 'center',
  },
  saveNameButtonDisabled: {
    opacity: 0.7,
  },
  saveNameText: {
    ...typography.bodyBold,
    color: colors.textPrimary,
  },

  // Generic info card
  card: {
    backgroundColor: colors.surface,
    borderRadius: borderRadius.lg,
    padding: spacing.lg,
    borderWidth: 1,
    borderColor: colors.border,
    marginBottom: spacing.md,
  },
  cardLabel: {
    ...typography.caption,
    color: colors.textMuted,
    textTransform: 'uppercase',
    letterSpacing: 1,
    marginBottom: spacing.xs,
  },
  cardSubtle: {
    ...typography.caption,
    color: colors.textSecondary,
    marginBottom: spacing.sm,
  },
  cardValue: {
    ...typography.h3,
    color: colors.textPrimary,
  },

  // Upgrade button inside subscription card (also re-used for share/link primary actions)
  upgradeButton: {
    backgroundColor: colors.primary,
    borderRadius: borderRadius.sm,
    paddingVertical: spacing.sm,
    paddingHorizontal: spacing.md,
    marginTop: spacing.md,
    alignItems: 'center',
  },
  upgradeText: {
    ...typography.bodyBold,
    color: colors.textPrimary,
  },

  // Share code display
  codeBox: {
    marginTop: spacing.md,
    alignItems: 'center',
  },
  codeValue: {
    ...typography.h1,
    color: colors.textPrimary,
    letterSpacing: 8,
    fontFamily: 'monospace',
  },
  codeHint: {
    ...typography.caption,
    color: colors.textMuted,
    marginTop: spacing.xs,
  },
  codeActionsRow: {
    flexDirection: 'row',
    marginTop: spacing.md,
    gap: spacing.sm,
    width: '100%',
  },
  codeActionBtn: {
    flex: 1,
    backgroundColor: colors.primary,
    borderRadius: borderRadius.sm,
    paddingVertical: spacing.sm,
    alignItems: 'center',
  },
  codeActionBtnSecondary: {
    backgroundColor: 'transparent',
    borderWidth: 1,
    borderColor: colors.border,
  },
  codeActionText: {
    ...typography.bodyBold,
    color: colors.textPrimary,
  },
  codeActionTextSecondary: {
    ...typography.bodyBold,
    color: colors.textSecondary,
  },

  // Link code input
  linkInputBox: {
    marginTop: spacing.md,
    alignItems: 'stretch',
  },
  codeInput: {
    backgroundColor: colors.surfaceLight,
    borderRadius: borderRadius.sm,
    padding: spacing.md,
    color: colors.textPrimary,
    ...typography.h2,
    borderWidth: 1,
    borderColor: colors.border,
    textAlign: 'center',
    letterSpacing: 6,
    fontFamily: 'monospace',
  },

  // Stats row
  statsRow: {
    flexDirection: 'row',
    marginBottom: spacing.md,
    gap: spacing.sm,
  },
  statCard: {
    flex: 1,
    backgroundColor: colors.surface,
    borderRadius: borderRadius.lg,
    padding: spacing.md,
    borderWidth: 1,
    borderColor: colors.border,
  },
  statCardLeft: {},
  statCardRight: {},
  statLabel: {
    ...typography.caption,
    color: colors.textMuted,
    textTransform: 'uppercase',
    letterSpacing: 1,
    marginBottom: spacing.xs,
  },
  statValue: {
    ...typography.captionBold,
    color: colors.textPrimary,
  },

  // Logout
  logoutButton: {
    marginTop: spacing.lg,
    paddingVertical: spacing.md,
    alignItems: 'center',
    borderRadius: borderRadius.sm,
    borderWidth: 1,
    borderColor: colors.error,
  },
  logoutText: {
    ...typography.bodyBold,
    color: colors.error,
  },
});
