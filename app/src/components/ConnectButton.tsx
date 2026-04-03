import React from 'react';
import {
  TouchableOpacity,
  View,
  Text,
  StyleSheet,
  ActivityIndicator,
} from 'react-native';
import {useTranslation} from 'react-i18next';
import {colors, typography, spacing, borderRadius} from '../theme';
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

  return (
    <TouchableOpacity
      onPress={onPress}
      disabled={isTransitioning}
      activeOpacity={0.8}
      style={styles.wrapper}>
      {/* Outer glow ring */}
      <View style={[styles.outerRing, {borderColor: buttonColor + '30'}]}>
        {/* Inner glow ring */}
        <View style={[styles.innerRing, {borderColor: buttonColor + '60'}]}>
          {/* Main button */}
          <View style={[styles.button, {backgroundColor: buttonColor + '20', borderColor: buttonColor}]}>
            {isTransitioning ? (
              <ActivityIndicator size="large" color={buttonColor} />
            ) : (
              <>
                <View style={[styles.powerIcon, {borderColor: buttonColor}]}>
                  <View style={[styles.powerLine, {backgroundColor: buttonColor}]} />
                </View>
              </>
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
  outerRing: {
    width: BUTTON_SIZE + 60,
    height: BUTTON_SIZE + 60,
    borderRadius: (BUTTON_SIZE + 60) / 2,
    borderWidth: 1,
    alignItems: 'center',
    justifyContent: 'center',
  },
  innerRing: {
    width: BUTTON_SIZE + 30,
    height: BUTTON_SIZE + 30,
    borderRadius: (BUTTON_SIZE + 30) / 2,
    borderWidth: 1,
    alignItems: 'center',
    justifyContent: 'center',
  },
  button: {
    width: BUTTON_SIZE,
    height: BUTTON_SIZE,
    borderRadius: BUTTON_SIZE / 2,
    borderWidth: 2,
    alignItems: 'center',
    justifyContent: 'center',
  },
  powerIcon: {
    width: 44,
    height: 44,
    borderRadius: 22,
    borderWidth: 3,
    borderTopColor: 'transparent',
    alignItems: 'center',
  },
  powerLine: {
    width: 3,
    height: 20,
    borderRadius: 2,
    marginTop: -8,
  },
  label: {
    ...typography.bodyBold,
    marginTop: spacing.md,
  },
});
