import React, {useState} from 'react';
import {
  View,
  Text,
  StyleSheet,
  SafeAreaView,
  ScrollView,
  TouchableOpacity,
  TextInput,
  ActivityIndicator,
} from 'react-native';
import {useTranslation} from 'react-i18next';
import {useNavigation} from '@react-navigation/native';
import type {NativeStackNavigationProp} from '@react-navigation/native-stack';
import {useAuthStore} from '../stores/authStore';
import {colors, typography, spacing, borderRadius} from '../theme';
import type {RootStackParamList} from '../navigation/RootNavigator';

type NavigationProp = NativeStackNavigationProp<RootStackParamList>;

export function AccountScreen() {
  const {t} = useTranslation();
  const navigation = useNavigation<NavigationProp>();
  const {user, isAuthenticated} = useAuthStore();

  if (!isAuthenticated) {
    return <LoginView />;
  }

  return (
    <SafeAreaView style={styles.container}>
      <ScrollView contentContainerStyle={styles.content}>
        {/* Subscription Card */}
        <View style={styles.subscriptionCard}>
          <Text style={styles.planLabel}>{t('account.subscription')}</Text>
          <Text style={styles.planName}>
            {user?.subscription_tier === 'premium'
              ? t('account.premiumPlan')
              : t('account.freePlan')}
          </Text>
          {(!user || user.subscription_tier === 'free') && (
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

        {/* Logout */}
        <TouchableOpacity
          style={styles.logoutButton}
          onPress={() => useAuthStore.getState().logout()}>
          <Text style={styles.logoutText}>{t('account.logout')}</Text>
        </TouchableOpacity>
      </ScrollView>
    </SafeAreaView>
  );
}

function LoginView() {
  const {t} = useTranslation();
  const {login, register, isLoading} = useAuthStore();
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [isRegisterMode, setIsRegisterMode] = useState(false);

  const handleSubmit = async () => {
    if (!email || !password) {
      setError('Email and password are required');
      return;
    }
    if (isRegisterMode && password.length < 8) {
      setError('Password must be at least 8 characters');
      return;
    }

    try {
      setError(null);
      if (isRegisterMode) {
        await register(email, password);
      } else {
        await login(email, password);
      }
    } catch (err: any) {
      const message =
        err?.response?.data?.error || 'Something went wrong. Please try again.';
      setError(message);
    }
  };

  return (
    <SafeAreaView style={styles.container}>
      <View style={styles.loginContent}>
        <Text style={styles.loginTitle}>
          {isRegisterMode ? t('account.register') : t('account.login')}
        </Text>

        {error && <Text style={styles.errorText}>{error}</Text>}

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
  },
  subscriptionCard: {
    backgroundColor: colors.surface,
    borderRadius: borderRadius.lg,
    padding: spacing.lg,
    borderWidth: 1,
    borderColor: colors.border,
    marginTop: spacing.md,
  },
  planLabel: {
    ...typography.caption,
    color: colors.textMuted,
    textTransform: 'uppercase',
    letterSpacing: 1,
  },
  planName: {
    ...typography.h2,
    color: colors.textPrimary,
    marginTop: spacing.xs,
  },
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
  logoutButton: {
    marginTop: spacing.xl,
    paddingVertical: spacing.md,
    alignItems: 'center',
  },
  logoutText: {
    ...typography.body,
    color: colors.error,
  },
  // Login view
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
