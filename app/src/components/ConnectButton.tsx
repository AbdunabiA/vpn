import React from 'react';
import {
  TouchableOpacity,
  View,
  Text,
  StyleSheet,
  ActivityIndicator,
} from 'react-native';
import {useTranslation} from 'react-i18next';
import {colors, typography, spacing} from '../theme';
import {useLayout} from '../hooks/useLayout';
import type {ConnectionState} from '../types/vpn';

interface ConnectButtonProps {
  state: ConnectionState;
  onPress: () => void;
}

const STATE_COLORS: Record<ConnectionState, string> = {
  disconnected: colors.primary,
  connecting: colors.warning,
  connected: colors.success,
  disconnecting: colors.warning,
  reconnecting: colors.warning,
  switching_protocol: colors.warning,
  error: colors.error,
};

export function ConnectButton({state, onPress}: ConnectButtonProps) {
  const {t} = useTranslation();
  const {scale} = useLayout();
  const isTransitioning = state === 'connecting' || state === 'disconnecting' || state === 'reconnecting' || state === 'switching_protocol';
  const buttonColor = STATE_COLORS[state];

  const label = {
    disconnected: t('connection.connect'),
    connecting: t('connection.connecting'),
    connected: t('connection.disconnect'),
    disconnecting: t('connection.disconnecting'),
    reconnecting: t('connection.reconnecting'),
    switching_protocol: t('connection.switchingProtocol', {defaultValue: 'Switching protocol...'}),
    error: t('common.retry'),
  }[state];

  const btnSize = BUTTON_SIZE * scale;
  const iconSize = 44 * scale;

  return (
    <TouchableOpacity
      onPress={onPress}
      disabled={isTransitioning}
      activeOpacity={0.8}
      style={styles.wrapper}>
      {/* Outer glow ring */}
      <View style={[styles.ring, {width: btnSize + 60 * scale, height: btnSize + 60 * scale, borderRadius: (btnSize + 60 * scale) / 2, borderColor: buttonColor + '30'}]}>
        {/* Inner glow ring */}
        <View style={[styles.ring, {width: btnSize + 30 * scale, height: btnSize + 30 * scale, borderRadius: (btnSize + 30 * scale) / 2, borderColor: buttonColor + '60'}]}>
          {/* Main button */}
          <View style={[styles.button, {width: btnSize, height: btnSize, borderRadius: btnSize / 2, backgroundColor: buttonColor + '20', borderColor: buttonColor}]}>
            {isTransitioning ? (
              <ActivityIndicator size="large" color={buttonColor} />
            ) : (
              <View style={[styles.powerIcon, {width: iconSize, height: iconSize, borderRadius: iconSize / 2, borderColor: buttonColor}]}>
                <View style={[styles.powerLine, {width: 3 * scale, height: 20 * scale, backgroundColor: buttonColor, marginTop: -8 * scale}]} />
              </View>
            )}
          </View>
        </View>
      </View>

      {/* Label below the button */}
      <Text style={[styles.label, {color: buttonColor}]}>{label}</Text>
    </TouchableOpacity>
  );
}

const BUTTON_SIZE = 140;

const styles = StyleSheet.create({
  wrapper: {
    alignItems: 'center',
  },
  ring: {
    borderWidth: 1,
    alignItems: 'center',
    justifyContent: 'center',
  },
  button: {
    borderWidth: 2,
    alignItems: 'center',
    justifyContent: 'center',
  },
  powerIcon: {
    borderWidth: 3,
    borderTopColor: 'transparent',
    alignItems: 'center',
  },
  powerLine: {
    borderRadius: 2,
  },
  label: {
    ...typography.bodyBold,
    marginTop: spacing.md,
  },
});
