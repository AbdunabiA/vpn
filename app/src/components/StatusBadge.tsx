import React from 'react';
import {View, Text, StyleSheet} from 'react-native';
import {useTranslation} from 'react-i18next';
import {colors, typography, spacing, borderRadius} from '../theme';
import type {ConnectionState} from '../types/vpn';

interface StatusBadgeProps {
  state: ConnectionState;
}

const STATE_COLORS: Record<ConnectionState, string> = {
  disconnected: colors.textMuted,
  connecting: colors.warning,
  connected: colors.success,
  disconnecting: colors.warning,
  reconnecting: colors.warning,
  switching_protocol: colors.warning,
  error: colors.error,
};

export function StatusBadge({state}: StatusBadgeProps) {
  const {t} = useTranslation();
  const color = STATE_COLORS[state];

  const label = {
    disconnected: t('connection.disconnected'),
    connecting: t('connection.connecting'),
    connected: t('connection.connected'),
    disconnecting: t('connection.disconnecting'),
    reconnecting: t('connection.reconnecting'),
    switching_protocol: t('connection.switchingProtocol', {defaultValue: 'Switching protocol...'}),
    error: t('common.error'),
  }[state];

  return (
    <View style={[styles.badge, {borderColor: color + '40'}]}>
      <View style={[styles.dot, {backgroundColor: color}]} />
      <Text style={[styles.text, {color}]}>{label}</Text>
    </View>
  );
}

const styles = StyleSheet.create({
  badge: {
    flexDirection: 'row',
    alignItems: 'center',
    paddingHorizontal: spacing.md,
    paddingVertical: spacing.xs,
    borderRadius: borderRadius.full,
    borderWidth: 1,
  },
  dot: {
    width: 8,
    height: 8,
    borderRadius: 4,
    marginRight: spacing.sm,
  },
  text: {
    ...typography.captionBold,
  },
});
