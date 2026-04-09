import React, {useMemo} from 'react';
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

const BUTTON_SIZE = 140;

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

  const sizes = useMemo(() => {
    const btn = BUTTON_SIZE * scale;
    const icon = 44 * scale;
    const outerSize = btn + 60 * scale;
    const innerSize = btn + 30 * scale;
    return {
      outerRing: {width: outerSize, height: outerSize, borderRadius: outerSize / 2},
      innerRing: {width: innerSize, height: innerSize, borderRadius: innerSize / 2},
      button: {width: btn, height: btn, borderRadius: btn / 2},
      icon: {width: icon, height: icon, borderRadius: icon / 2},
      line: {width: 3 * scale, height: 20 * scale, marginTop: -8 * scale},
    };
  }, [scale]);

  return (
    <TouchableOpacity
      onPress={onPress}
      disabled={isTransitioning}
      activeOpacity={0.8}
      style={styles.wrapper}>
      {/* Outer glow ring */}
      <View style={[styles.ring, sizes.outerRing, {borderColor: buttonColor + '30'}]}>
        {/* Inner glow ring */}
        <View style={[styles.ring, sizes.innerRing, {borderColor: buttonColor + '60'}]}>
          {/* Main button */}
          <View style={[styles.button, sizes.button, {backgroundColor: buttonColor + '20', borderColor: buttonColor}]}>
            {isTransitioning ? (
              <ActivityIndicator size="large" color={buttonColor} />
            ) : (
              <View style={[styles.powerIcon, sizes.icon, {borderColor: buttonColor}]}>
                <View style={[styles.powerLine, sizes.line, {backgroundColor: buttonColor}]} />
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
