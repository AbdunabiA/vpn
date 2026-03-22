import React from 'react';
import {View, Text, StyleSheet} from 'react-native';
import {colors, typography, spacing} from '../theme';

interface SpeedIndicatorProps {
  speedUp: number; // bytes per second
  speedDown: number;
  bytesUp: number;
  bytesDown: number;
}

function formatSpeed(bytesPerSec: number): string {
  if (bytesPerSec < 1024) return `${bytesPerSec.toFixed(0)} B/s`;
  if (bytesPerSec < 1024 * 1024) return `${(bytesPerSec / 1024).toFixed(1)} KB/s`;
  return `${(bytesPerSec / (1024 * 1024)).toFixed(1)} MB/s`;
}

function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  if (bytes < 1024 * 1024 * 1024) return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
  return `${(bytes / (1024 * 1024 * 1024)).toFixed(2)} GB`;
}

export function SpeedIndicator({speedUp, speedDown, bytesUp, bytesDown}: SpeedIndicatorProps) {
  return (
    <View style={styles.container}>
      {/* Download */}
      <View style={styles.statColumn}>
        <Text style={styles.arrow}>↓</Text>
        <Text style={styles.speed}>{formatSpeed(speedDown)}</Text>
        <Text style={styles.total}>{formatBytes(bytesDown)}</Text>
      </View>

      {/* Divider */}
      <View style={styles.divider} />

      {/* Upload */}
      <View style={styles.statColumn}>
        <Text style={styles.arrow}>↑</Text>
        <Text style={styles.speed}>{formatSpeed(speedUp)}</Text>
        <Text style={styles.total}>{formatBytes(bytesUp)}</Text>
      </View>
    </View>
  );
}

const styles = StyleSheet.create({
  container: {
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'center',
    paddingVertical: spacing.md,
    paddingHorizontal: spacing.xl,
  },
  statColumn: {
    alignItems: 'center',
    flex: 1,
  },
  arrow: {
    ...typography.h3,
    color: colors.primary,
  },
  speed: {
    ...typography.mono,
    color: colors.textPrimary,
    marginTop: spacing.xs,
  },
  total: {
    ...typography.caption,
    color: colors.textMuted,
    marginTop: 2,
  },
  divider: {
    width: 1,
    height: 50,
    backgroundColor: colors.border,
    marginHorizontal: spacing.lg,
  },
});
