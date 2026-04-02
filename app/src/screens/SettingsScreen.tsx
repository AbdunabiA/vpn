import React from 'react';
import {
  View,
  Text,
  Switch,
  StyleSheet,
  SafeAreaView,
  ScrollView,
  TouchableOpacity,
} from 'react-native';
import {useTranslation} from 'react-i18next';
import {useNavigation} from '@react-navigation/native';
import type {NativeStackNavigationProp} from '@react-navigation/native-stack';
import {useSettingsStore} from '../stores/settingsStore';
import {colors, typography, spacing, borderRadius} from '../theme';
import type {VpnProtocol} from '../types/vpn';
import type {RootStackParamList} from '../navigation/RootNavigator';

function SettingRow({
  label,
  description,
  right,
}: {
  label: string;
  description?: string;
  right: React.ReactNode;
}) {
  return (
    <View style={styles.row}>
      <View style={styles.rowLeft}>
        <Text style={styles.rowLabel}>{label}</Text>
        {description && <Text style={styles.rowDesc}>{description}</Text>}
      </View>
      {right}
    </View>
  );
}

function NavRow({
  label,
  description,
  onPress,
  badge,
}: {
  label: string;
  description?: string;
  onPress: () => void;
  badge?: string;
}) {
  return (
    <TouchableOpacity style={styles.row} onPress={onPress} activeOpacity={0.7}>
      <View style={styles.rowLeft}>
        <Text style={styles.rowLabel}>{label}</Text>
        {description && <Text style={styles.rowDesc}>{description}</Text>}
      </View>
      <View style={styles.navRight}>
        {badge ? <Text style={styles.navBadge}>{badge}</Text> : null}
        <Text style={styles.navChevron}>›</Text>
      </View>
    </TouchableOpacity>
  );
}

const PROTOCOLS: {value: VpnProtocol; label: string}[] = [
  {value: 'auto', label: 'Auto'},
  {value: 'vless-reality', label: 'VLESS+REALITY'},
  {value: 'amneziawg', label: 'AmneziaWG'},
  {value: 'websocket', label: 'WebSocket (CDN)'},
];

export function SettingsScreen() {
  const {t} = useTranslation();
  const navigation =
    useNavigation<NativeStackNavigationProp<RootStackParamList>>();
  const {
    protocol,
    killSwitch,
    dnsOverHttps,
    autoReconnect,
    excludedApps,
    excludedDomains,
    setProtocol,
    setKillSwitch,
    setDnsOverHttps,
    setAutoReconnect,
  } = useSettingsStore();

  // Badge shows count of excluded items so the user knows the feature is active
  const splitBadge = (() => {
    const count = excludedApps.length + excludedDomains.length;
    return count > 0 ? String(count) : undefined;
  })();

  return (
    <SafeAreaView style={styles.container}>
      <ScrollView contentContainerStyle={styles.content}>
        {/* Protocol Selection */}
        <Text style={styles.sectionTitle}>{t('settings.protocol')}</Text>
        <View style={styles.card}>
          {PROTOCOLS.map((p) => (
            <TouchableOpacity
              key={p.value}
              style={[
                styles.protocolOption,
                protocol === p.value && styles.protocolSelected,
              ]}
              onPress={() => setProtocol(p.value)}>
              <Text
                style={[
                  styles.protocolText,
                  protocol === p.value && styles.protocolTextSelected,
                ]}>
                {p.value === 'auto' ? t('settings.protocolAuto') : p.label}
              </Text>
              {protocol === p.value && <Text style={styles.checkmark}>✓</Text>}
            </TouchableOpacity>
          ))}
        </View>

        {/* Toggles */}
        <Text style={styles.sectionTitle}>{t('settings.title')}</Text>
        <View style={styles.card}>
          <SettingRow
            label={t('settings.killSwitch')}
            description={t('settings.killSwitchDesc')}
            right={
              <Switch
                value={killSwitch}
                onValueChange={setKillSwitch}
                trackColor={{true: colors.primary, false: colors.surfaceLight}}
                thumbColor={colors.textPrimary}
              />
            }
          />
          <View style={styles.separator} />
          <NavRow
            label={t('settings.splitTunneling')}
            description={t('splitTunnel.description')}
            onPress={() => navigation.navigate('SplitTunnel')}
            badge={splitBadge}
          />
          <View style={styles.separator} />
          <SettingRow
            label={t('settings.dns')}
            right={
              <Switch
                value={dnsOverHttps}
                onValueChange={setDnsOverHttps}
                trackColor={{true: colors.primary, false: colors.surfaceLight}}
                thumbColor={colors.textPrimary}
              />
            }
          />
          <View style={styles.separator} />
          <SettingRow
            label={t('settings.autoReconnect')}
            description={t('settings.autoReconnectDesc')}
            right={
              <Switch
                value={autoReconnect}
                onValueChange={setAutoReconnect}
                trackColor={{true: colors.primary, false: colors.surfaceLight}}
                thumbColor={colors.textPrimary}
              />
            }
          />
        </View>
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
  },
  sectionTitle: {
    ...typography.captionBold,
    color: colors.textMuted,
    textTransform: 'uppercase',
    letterSpacing: 1,
    marginTop: spacing.lg,
    marginBottom: spacing.sm,
    marginLeft: spacing.xs,
  },
  card: {
    backgroundColor: colors.surface,
    borderRadius: borderRadius.md,
    overflow: 'hidden',
    borderWidth: 1,
    borderColor: colors.border,
  },
  row: {
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'space-between',
    paddingHorizontal: spacing.md,
    paddingVertical: spacing.md,
  },
  rowLeft: {
    flex: 1,
    marginRight: spacing.md,
  },
  rowLabel: {
    ...typography.body,
    color: colors.textPrimary,
  },
  rowDesc: {
    ...typography.caption,
    color: colors.textMuted,
    marginTop: 2,
  },
  separator: {
    height: 1,
    backgroundColor: colors.border,
    marginLeft: spacing.md,
  },
  protocolOption: {
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'space-between',
    paddingHorizontal: spacing.md,
    paddingVertical: spacing.md,
    borderBottomWidth: 1,
    borderBottomColor: colors.border,
  },
  protocolSelected: {
    backgroundColor: colors.primary + '10',
  },
  protocolText: {
    ...typography.body,
    color: colors.textPrimary,
  },
  protocolTextSelected: {
    color: colors.primary,
    fontWeight: '600',
  },
  checkmark: {
    color: colors.primary,
    fontSize: 18,
    fontWeight: '700',
  },
  navRight: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 6,
  },
  navBadge: {
    ...typography.captionBold,
    color: colors.primary,
    backgroundColor: colors.primary + '20',
    paddingHorizontal: 7,
    paddingVertical: 2,
    borderRadius: borderRadius.full,
    overflow: 'hidden',
  },
  navChevron: {
    fontSize: 22,
    color: colors.textMuted,
    lineHeight: 24,
  },
});
