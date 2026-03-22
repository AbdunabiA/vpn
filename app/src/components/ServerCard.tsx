import React from 'react';
import {TouchableOpacity, View, Text, StyleSheet} from 'react-native';
import {colors, typography, spacing, borderRadius} from '../theme';
import type {Server} from '../types/api';

interface ServerCardProps {
  server: Server;
  isSelected: boolean;
  isFavorite: boolean;
  onPress: () => void;
  onFavoritePress: () => void;
}

// Country code to flag emoji
function getFlagEmoji(countryCode: string): string {
  return countryCode
    .toUpperCase()
    .split('')
    .map((char) => String.fromCodePoint(127397 + char.charCodeAt(0)))
    .join('');
}

function getLoadColor(percent: number): string {
  if (percent < 40) return colors.success;
  if (percent < 70) return colors.warning;
  return colors.error;
}

export function ServerCard({
  server,
  isSelected,
  isFavorite,
  onPress,
  onFavoritePress,
}: ServerCardProps) {
  return (
    <TouchableOpacity
      style={[styles.card, isSelected && styles.cardSelected]}
      onPress={onPress}
      activeOpacity={0.7}>
      <View style={styles.row}>
        {/* Flag and location */}
        <Text style={styles.flag}>{getFlagEmoji(server.country_code)}</Text>
        <View style={styles.info}>
          <Text style={styles.city}>{server.city}</Text>
          <Text style={styles.country}>{server.country}</Text>
        </View>

        {/* Load indicator */}
        <View style={styles.loadContainer}>
          <View style={styles.loadBarBg}>
            <View
              style={[
                styles.loadBarFill,
                {
                  width: `${server.load_percent}%`,
                  backgroundColor: getLoadColor(server.load_percent),
                },
              ]}
            />
          </View>
          <Text style={[styles.loadText, {color: getLoadColor(server.load_percent)}]}>
            {server.load_percent}%
          </Text>
        </View>

        {/* Favorite button */}
        <TouchableOpacity onPress={onFavoritePress} style={styles.favoriteBtn}>
          <Text style={styles.favoriteIcon}>{isFavorite ? '★' : '☆'}</Text>
        </TouchableOpacity>
      </View>
    </TouchableOpacity>
  );
}

const styles = StyleSheet.create({
  card: {
    backgroundColor: colors.surface,
    borderRadius: borderRadius.md,
    padding: spacing.md,
    marginHorizontal: spacing.md,
    marginVertical: spacing.xs,
    borderWidth: 1,
    borderColor: colors.border,
  },
  cardSelected: {
    borderColor: colors.primary,
    backgroundColor: colors.primary + '10',
  },
  row: {
    flexDirection: 'row',
    alignItems: 'center',
  },
  flag: {
    fontSize: 28,
    marginRight: spacing.sm,
  },
  info: {
    flex: 1,
  },
  city: {
    ...typography.bodyBold,
    color: colors.textPrimary,
  },
  country: {
    ...typography.caption,
    color: colors.textSecondary,
  },
  loadContainer: {
    alignItems: 'flex-end',
    marginRight: spacing.sm,
  },
  loadBarBg: {
    width: 50,
    height: 4,
    backgroundColor: colors.surfaceLight,
    borderRadius: 2,
    overflow: 'hidden',
  },
  loadBarFill: {
    height: '100%',
    borderRadius: 2,
  },
  loadText: {
    ...typography.caption,
    marginTop: 2,
  },
  favoriteBtn: {
    padding: spacing.xs,
  },
  favoriteIcon: {
    fontSize: 20,
    color: colors.warning,
  },
});
