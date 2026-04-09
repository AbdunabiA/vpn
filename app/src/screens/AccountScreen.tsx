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
} from 'react-native';
import {useTranslation} from 'react-i18next';
import {useNavigation} from '@react-navigation/native';
import type {NativeStackNavigationProp} from '@react-navigation/native-stack';
import {useAuthStore} from '../stores/authStore';
import {useLayout} from '../hooks/useLayout';
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
  const {user, isAuthenticated, fetchAccount, updateProfile} = useAuthStore();
  const {contentMaxWidth, scale} = useLayout();

  const [isEditingName, setIsEditingName] = useState(false);
  const [nameInput, setNameInput] = useState('');
  const [nameSaving, setNameSaving] = useState(false);
  const [nameError, setNameError] = useState<string | null>(null);
  const [activeDevices, setActiveDevices] = useState<number | null>(null);

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
      const api = (await import('../services/api')).default;
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
    Alert.alert(
      t('account.logout'),
      '',
      [
        {text: t('common.cancel'), style: 'cancel'},
        {
          text: t('account.logout'),
          style: 'destructive',
          onPress: () => useAuthStore.getState().logout(),
        },
      ],
    );
  }

  if (!isAuthenticated) {
    return <LoginView />;
  }

  const displayName = user?.full_name || '?';
  const tier = user?.subscription_tier ?? 'free';
  const createdAt = user?.created_at ? formatDate(user.created_at) : '—';

  return (
    <SafeAreaView style={styles.container}>
      <ScrollView
        contentContainerStyle={[styles.content, contentMaxWidth ? {maxWidth: contentMaxWidth, alignSelf: 'center', width: '100%'} : undefined]}
        showsVerticalScrollIndicator={false}>

        {/* Avatar + Name header */}
        <View style={styles.headerCard}>
          <View style={[styles.avatarCircle, {width: 72 * scale, height: 72 * scale}]}>
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
              {nameError && (
                <Text style={styles.fieldError}>{nameError}</Text>
              )}
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

        {/* Email status card */}
        <View style={styles.card}>
          <Text style={styles.cardLabel}>{t('account.email')}</Text>
          <View style={styles.cardRow}>
            <View style={styles.verifiedBadge}>
              <Text style={styles.verifiedText}>{t('account.emailVerified')}</Text>
            </View>
          </View>
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

// ---- LoginView ----

function LoginView() {
  const {t} = useTranslation();
  const {contentMaxWidth} = useLayout();
  const {login, register, isLoading} = useAuthStore();
  const [name, setName] = useState('');
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [isRegisterMode, setIsRegisterMode] = useState(false);

  const handleSubmit = async () => {
    if (!email || !password) {
      setError('Email and password are required');
      return;
    }
    if (isRegisterMode && name.trim().length < 2) {
      setError(t('register.nameRequired'));
      return;
    }
    if (isRegisterMode && password.length < 8) {
      setError('Password must be at least 8 characters');
      return;
    }

    try {
      setError(null);
      if (isRegisterMode) {
        await register(email, password, name.trim());
      } else {
        await login(email, password);
      }
    } catch (err: unknown) {
      const anyErr = err as {response?: {data?: {error?: string}}};
      const message =
        anyErr?.response?.data?.error || 'Something went wrong. Please try again.';
      setError(message);
    }
  };

  return (
    <SafeAreaView style={styles.container}>
      <View style={[styles.loginContent, contentMaxWidth ? {maxWidth: contentMaxWidth, alignSelf: 'center', width: '100%'} : undefined]}>
        <Text style={styles.loginTitle}>
          {isRegisterMode ? t('account.register') : t('account.login')}
        </Text>

        {error && <Text style={styles.errorText}>{error}</Text>}

        {isRegisterMode && (
          <TextInput
            style={styles.input}
            placeholder={t('register.namePlaceholder')}
            placeholderTextColor={colors.textMuted}
            value={name}
            onChangeText={setName}
            autoCapitalize="words"
            editable={!isLoading}
          />
        )}

        <TextInput
          style={styles.input}
          placeholder={t('account.email')}
          placeholderTextColor={colors.textMuted}
          value={email}
          onChangeText={setEmail}
          keyboardType="email-address"
          autoCapitalize="none"
          editable={!isLoading}
        />

        <TextInput
          style={styles.input}
          placeholder={t('account.password')}
          placeholderTextColor={colors.textMuted}
          value={password}
          onChangeText={setPassword}
          secureTextEntry
          editable={!isLoading}
        />

        <TouchableOpacity
          style={[styles.loginButton, isLoading && styles.loginButtonDisabled]}
          onPress={handleSubmit}
          disabled={isLoading}>
          {isLoading ? (
            <ActivityIndicator color={colors.textPrimary} />
          ) : (
            <Text style={styles.loginButtonText}>
              {isRegisterMode ? t('account.register') : t('account.login')}
            </Text>
          )}
        </TouchableOpacity>

        <TouchableOpacity
          style={styles.registerLink}
          onPress={() => {
            setIsRegisterMode(!isRegisterMode);
            setError(null);
            setName('');
          }}>
          <Text style={styles.registerText}>
            {isRegisterMode ? t('account.login') : t('account.register')}
          </Text>
        </TouchableOpacity>
      </View>
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
  cardValue: {
    ...typography.h3,
    color: colors.textPrimary,
  },
  cardRow: {
    flexDirection: 'row',
    alignItems: 'center',
  },
  verifiedBadge: {
    backgroundColor: colors.successDark,
    borderRadius: borderRadius.sm,
    paddingVertical: spacing.xs,
    paddingHorizontal: spacing.sm,
  },
  verifiedText: {
    ...typography.captionBold,
    color: colors.textPrimary,
  },

  // Upgrade button inside subscription card
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

  // ---- LoginView styles ----
  loginContent: {
    flex: 1,
    justifyContent: 'center',
    paddingHorizontal: spacing.xl,
  },
  loginTitle: {
    ...typography.h1,
    color: colors.textPrimary,
    textAlign: 'center',
    marginBottom: spacing.xl,
  },
  errorText: {
    ...typography.caption,
    color: colors.error,
    textAlign: 'center',
    marginBottom: spacing.md,
  },
  input: {
    backgroundColor: colors.surface,
    borderRadius: borderRadius.sm,
    padding: spacing.md,
    color: colors.textPrimary,
    ...typography.body,
    marginBottom: spacing.md,
    borderWidth: 1,
    borderColor: colors.border,
  },
  loginButton: {
    backgroundColor: colors.primary,
    borderRadius: borderRadius.sm,
    paddingVertical: spacing.md,
    alignItems: 'center',
    marginTop: spacing.sm,
  },
  loginButtonDisabled: {
    opacity: 0.7,
  },
  loginButtonText: {
    ...typography.bodyBold,
    color: colors.textPrimary,
  },
  registerLink: {
    marginTop: spacing.lg,
    alignItems: 'center',
  },
  registerText: {
    ...typography.body,
    color: colors.primary,
  },
});
