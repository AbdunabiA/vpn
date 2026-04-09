import React, {useState} from 'react';
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
import {createCheckoutSession} from '../services/payment';
import {colors, typography, spacing, borderRadius} from '../theme';
import type {Subscription} from '../types/api';

type Plan = 'free' | 'premium' | 'ultimate';

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
    price: '$9.99',
    period: '/mo',
    features: [
      '5 devices',
      'Unlimited data',
      '40+ server locations',
      'High speed',
      'Kill switch',
    ],
    highlighted: true,
  },
  {
    id: 'ultimate',
    price: '$19.99',
    period: '/mo',
    features: [
      '10 devices',
      'Unlimited data',
      '80+ server locations',
      'Maximum speed',
      'Kill switch',
      'Dedicated IP',
      'Priority support',
    ],
    highlighted: false,
  },
];

interface PlanCardProps {
  plan: PlanConfig;
  isCurrent: boolean;
  isUpgrading: boolean;
  onUpgrade: (plan: Plan) => void;
}

function PlanCard({plan, isCurrent, isUpgrading, onUpgrade}: PlanCardProps) {
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
          disabled={isUpgrading}
          activeOpacity={0.8}
          accessibilityRole="button"
          accessibilityLabel={t('payment.upgrade', {plan: plan.id})}>
          {isUpgrading ? (
            <ActivityIndicator color={colors.textPrimary} />
          ) : (
            <Text style={styles.upgradeButtonText}>{t('payment.upgrade')}</Text>
          )}
        </TouchableOpacity>
      ) : null}
    </View>
  );
}

export function PaymentScreen() {
  const {t} = useTranslation();
  const {tabletContentStyle} = useLayout();
  const {data: subscription, isLoading} = useSubscription();
  const [upgradingPlan, setUpgradingPlan] = useState<Plan | null>(null);

  const currentPlan = (subscription as Subscription | undefined)?.plan ?? 'free';

  const handleUpgrade = async (plan: Plan) => {
    setUpgradingPlan(plan);
    try {
      const session = await createCheckoutSession(plan);
      await Linking.openURL(session.url);
    } catch {
      Alert.alert(
        t('payment.errorTitle'),
        t('payment.errorMessage'),
        [{text: t('common.retry'), style: 'default'}],
      );
    } finally {
      setUpgradingPlan(null);
    }
  };

  return (
    <SafeAreaView style={styles.container}>
      <ScrollView contentContainerStyle={[styles.content, tabletContentStyle]} showsVerticalScrollIndicator={false}>
        <Text style={styles.screenTitle}>{t('payment.title')}</Text>
        <Text style={styles.screenSubtitle}>{t('payment.subtitle')}</Text>

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
              isUpgrading={upgradingPlan === plan.id}
              onUpgrade={handleUpgrade}
            />
          ))
        )}

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
    marginBottom: spacing.xl,
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
  disclaimer: {
    ...typography.caption,
    color: colors.textMuted,
    textAlign: 'center',
    marginTop: spacing.md,
  },
});
