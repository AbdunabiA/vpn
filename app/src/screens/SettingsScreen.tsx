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
import {useSettingsStore} from '../stores/settingsStore';
import {colors, typography, spacing, borderRadius} from '../theme';
import type {VpnProtocol} from '../types/vpn';

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

const PROTOCOLS: {value: VpnProtocol; label: string}[] = [
  {value: 'auto', label: 'Auto'},
  {value: 'vless-reality', label: 'VLESS+REALITY'},
  {value: 'amneziawg', label: 'AmneziaWG'},
  {value: 'websocket', label: 'WebSocket (CDN)'},
];

export function SettingsScreen() {
  const {t} = useTranslation();
  const {
    protocol,
    killSwitch,
    splitTunneling,
    dnsOverHttps,
    setProtocol,
    setKillSwitch,
    setSplitTunneling,
    setDnsOverHttps,
  } = useSettingsStore();

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
          <SettingRow
            label={t('settings.splitTunneling')}
            right={
              <Switch
                value={splitTunneling}
                onValueChange={setSplitTunneling}
                trackColor={{true: colors.primary, false: colors.surfaceLight}}
                thumbColor={colors.textPrimary}
              />
            }
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
});
