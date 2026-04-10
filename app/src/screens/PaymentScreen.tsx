import React from 'react';
import {
  View,
  Text,
  StyleSheet,
  SafeAreaView,
  ScrollView,
  TouchableOpacity,
  ActivityIndicator,
  Linking,
  Alert,
} from 'react-native';
import {useTranslation} from 'react-i18next';
import {useSubscription} from '../hooks/useSubscription';
import {useLayout} from '../hooks/useLayout';
import {useAuthStore} from '../stores/authStore';
import {colors, typography, spacing, borderRadius} from '../theme';
import type {Subscription} from '../types/api';

type Plan = 'free' | 'premium' | 'ultimate';

// Telegram handle that receives premium requests. When Stripe is wired up
// we'll replace this flow with in-app checkout; for now users contact support
// manually and an admin activates their subscription through the admin panel.
const SUPPORT_TELEGRAM = 'flawlssr';

interface PlanConfig {
  id: Plan;
  price: string;
  period: string;
  features: string[];
  highlighted: boolean;
}

const PLANS: PlanConfig[] = [
  {
    id: 'free',
    price: '$0',
    period: '',
    features: [
      '1 device',
      '10 GB / month',
      '5 server locations',
      'Standard speed',
    ],
    highlighted: false,
  },
  {
    id: 'premium',
    price: '$4.99',
    period: '/mo',
    features: [
      '5 devices',
      'Unlimited data',
      '40+ server locations',
      'High speed',
      'Kill switch',
      'No ads',
    ],
    highlighted: true,
  },
  {
    id: 'ultimate',
    price: '$9.99',
    period: '/mo',
    features: [
      '10 devices',
      'Unlimited data',
      '80+ server locations',
      'Maximum speed',
      'Kill switch',
      'No ads',
      'Priority support',
    ],
    highlighted: false,
  },
];

interface PlanCardProps {
  plan: PlanConfig;
  isCurrent: boolean;
  onUpgrade: (plan: Plan) => void;
}

function PlanCard({plan, isCurrent, onUpgrade}: PlanCardProps) {
  const {t} = useTranslation();

  return (
    <View
      style={[
        styles.planCard,
        plan.highlighted && styles.planCardHighlighted,
        isCurrent && styles.planCardCurrent,
      ]}
      accessibilityRole="none">
      {plan.highlighted && (
        <View style={styles.popularBadge}>
          <Text style={styles.popularBadgeText}>{t('payment.mostPopular')}</Text>
        </View>
      )}

      <Text style={styles.planName}>{t(`payment.plans.${plan.id}.name`)}</Text>
      <View style={styles.priceRow}>
        <Text style={styles.planPrice}>{plan.price}</Text>
        {plan.period ? (
          <Text style={styles.planPeriod}>{plan.period}</Text>
        ) : null}
      </View>

      <View style={styles.featureList}>
        {plan.features.map((feature) => (
          <View key={feature} style={styles.featureRow}>
            <Text style={styles.featureCheck}>✓</Text>
            <Text style={styles.featureText}>{feature}</Text>
          </View>
        ))}
      </View>

      {isCurrent ? (
        <View
          style={styles.currentBadge}
          accessibilityLabel={t('payment.currentPlan')}>
          <Text style={styles.currentBadgeText}>✓ {t('payment.currentPlan')}</Text>
        </View>
      ) : plan.id !== 'free' ? (
        <TouchableOpacity
          style={[styles.upgradeButton, plan.highlighted && styles.upgradeButtonHighlighted]}
          onPress={() => onUpgrade(plan.id)}
          activeOpacity={0.8}
          accessibilityRole="button"
          accessibilityLabel={t('payment.contactSupport')}>
          <Text style={styles.upgradeButtonText}>{t('payment.contactSupport')}</Text>
        </TouchableOpacity>
      ) : null}
    </View>
  );
}

export function PaymentScreen() {
  const {t} = useTranslation();
  const {tabletContentStyle} = useLayout();
  const {data: subscription, isLoading} = useSubscription();
  const user = useAuthStore(s => s.user);

  const currentPlan = (subscription as Subscription | undefined)?.plan ?? 'free';
  const userId = user?.id ?? '';
  const shortId = userId.substring(0, 8);

  const openTelegram = async (plan: Plan) => {
    if (!userId) {
      Alert.alert(t('common.error'), t('payment.idMissing'));
      return;
    }
    const planName = plan.charAt(0).toUpperCase() + plan.slice(1);
    const message = t('payment.telegramMessage', {plan: planName, id: userId});
    const encoded = encodeURIComponent(message);
    const url = `https://t.me/${SUPPORT_TELEGRAM}?text=${encoded}`;
    try {
      await Linking.openURL(url);
    } catch {
      Alert.alert(
        t('payment.errorTitle'),
        t('payment.telegramError', {handle: `@${SUPPORT_TELEGRAM}`}),
      );
    }
  };

  return (
    <SafeAreaView style={styles.container}>
      <ScrollView contentContainerStyle={[styles.content, tabletContentStyle]} showsVerticalScrollIndicator={false}>
        <Text style={styles.screenTitle}>{t('payment.title')}</Text>
        <Text style={styles.screenSubtitle}>{t('payment.subtitle')}</Text>

        {/* User ID card — visible so users see their identifier before contacting
            support. The full ID is also injected into the Telegram prefilled message,
            so users don't need to type it manually. */}
        {userId ? (
          <View style={styles.idCard}>
            <Text style={styles.idLabel}>{t('payment.yourId')}</Text>
            <Text style={styles.idValue}>{shortId}…</Text>
          </View>
        ) : null}

        {isLoading ? (
          <View style={styles.loadingContainer}>
            <ActivityIndicator size="large" color={colors.primary} />
          </View>
        ) : (
          PLANS.map((plan) => (
            <PlanCard
              key={plan.id}
              plan={plan}
              isCurrent={currentPlan === plan.id}
              onUpgrade={openTelegram}
            />
          ))
        )}

        <View style={styles.supportCard}>
          <Text style={styles.supportTitle}>{t('payment.howItWorksTitle')}</Text>
          <Text style={styles.supportText}>{t('payment.howItWorksBody')}</Text>
        </View>

        <Text style={styles.disclaimer}>{t('payment.disclaimer')}</Text>
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
  screenTitle: {
    ...typography.h2,
    color: colors.textPrimary,
    textAlign: 'center',
    marginTop: spacing.md,
    marginBottom: spacing.xs,
  },
  screenSubtitle: {
    ...typography.body,
    color: colors.textSecondary,
    textAlign: 'center',
    marginBottom: spacing.lg,
  },
  idCard: {
    backgroundColor: colors.surface,
    borderRadius: borderRadius.md,
    borderWidth: 1,
    borderColor: colors.border,
    paddingVertical: spacing.md,
    paddingHorizontal: spacing.lg,
    marginBottom: spacing.lg,
    alignItems: 'center',
  },
  idLabel: {
    ...typography.caption,
    color: colors.textMuted,
    marginBottom: 2,
  },
  idValue: {
    ...typography.bodyBold,
    color: colors.textPrimary,
    fontFamily: 'monospace',
    letterSpacing: 1,
  },
  loadingContainer: {
    paddingVertical: spacing.xxl,
    alignItems: 'center',
  },
  planCard: {
    backgroundColor: colors.surface,
    borderRadius: borderRadius.lg,
    padding: spacing.lg,
    borderWidth: 1,
    borderColor: colors.border,
    marginBottom: spacing.md,
  },
  planCardHighlighted: {
    borderColor: colors.primary,
    backgroundColor: colors.primary + '08',
  },
  planCardCurrent: {
    borderColor: colors.success,
  },
  popularBadge: {
    alignSelf: 'flex-start',
    backgroundColor: colors.primary,
    borderRadius: borderRadius.full,
    paddingHorizontal: spacing.sm,
    paddingVertical: 2,
    marginBottom: spacing.sm,
  },
  popularBadgeText: {
    ...typography.caption,
    color: colors.textPrimary,
    fontWeight: '600',
  },
  planName: {
    ...typography.h3,
    color: colors.textPrimary,
    marginBottom: spacing.xs,
  },
  priceRow: {
    flexDirection: 'row',
    alignItems: 'baseline',
    marginBottom: spacing.md,
  },
  planPrice: {
    ...typography.h1,
    color: colors.textPrimary,
  },
  planPeriod: {
    ...typography.body,
    color: colors.textMuted,
    marginLeft: spacing.xs,
  },
  featureList: {
    marginBottom: spacing.md,
  },
  featureRow: {
    flexDirection: 'row',
    alignItems: 'center',
    marginBottom: spacing.xs,
  },
  featureCheck: {
    ...typography.body,
    color: colors.success,
    marginRight: spacing.sm,
    width: 16,
  },
  featureText: {
    ...typography.body,
    color: colors.textSecondary,
    flex: 1,
  },
  upgradeButton: {
    backgroundColor: colors.surfaceLight,
    borderRadius: borderRadius.sm,
    paddingVertical: spacing.sm,
    alignItems: 'center',
    marginTop: spacing.xs,
  },
  upgradeButtonHighlighted: {
    backgroundColor: colors.primary,
  },
  upgradeButtonText: {
    ...typography.bodyBold,
    color: colors.textPrimary,
  },
  currentBadge: {
    borderWidth: 1,
    borderColor: colors.success,
    borderRadius: borderRadius.sm,
    paddingVertical: spacing.sm,
    alignItems: 'center',
    marginTop: spacing.xs,
  },
  currentBadgeText: {
    ...typography.bodyBold,
    color: colors.success,
  },
  supportCard: {
    backgroundColor: colors.surface,
    borderRadius: borderRadius.md,
    padding: spacing.md,
    marginTop: spacing.sm,
    marginBottom: spacing.md,
    borderLeftWidth: 3,
    borderLeftColor: colors.primary,
  },
  supportTitle: {
    ...typography.bodyBold,
    color: colors.textPrimary,
    marginBottom: spacing.xs,
  },
  supportText: {
    ...typography.caption,
    color: colors.textSecondary,
    lineHeight: 18,
  },
  disclaimer: {
    ...typography.caption,
    color: colors.textMuted,
    textAlign: 'center',
    marginTop: spacing.md,
  },
});
