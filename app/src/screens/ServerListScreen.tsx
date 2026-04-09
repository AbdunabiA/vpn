import React, {useMemo} from 'react';
import {
  View,
  Text,
  FlatList,
  StyleSheet,
  SafeAreaView,
  ActivityIndicator,
} from 'react-native';
import {useTranslation} from 'react-i18next';
import {useNavigation} from '@react-navigation/native';

import {ServerCard} from '../components/ServerCard';
import {useServerList} from '../hooks/useServerList';
import {useServerStore} from '../stores/serverStore';
import {useLayout} from '../hooks/useLayout';
import {colors, typography, spacing} from '../theme';
import type {Server} from '../types/api';

export function ServerListScreen() {
  const {t} = useTranslation();
  const navigation = useNavigation();
  const {data: servers, isLoading, error} = useServerList();
  const {selectedServer, favoriteIds, selectServer, toggleFavorite} = useServerStore();
  const {contentMaxWidth} = useLayout();

  // Group servers by region
  const sections = useMemo(() => {
    if (!servers) return [];

    const grouped: Record<string, Server[]> = {};
    for (const server of servers) {
      if (!grouped[server.region]) {
        grouped[server.region] = [];
      }
      grouped[server.region].push(server);
    }

    return Object.entries(grouped).map(([region, items]) => ({
      region,
      data: items.sort((a, b) => a.load_percent - b.load_percent),
    }));
  }, [servers]);

  const handleSelectServer = (server: Server) => {
    selectServer(server);
    navigation.goBack();
  };

  if (isLoading) {
    return (
      <SafeAreaView style={styles.container}>
        <View style={styles.center}>
          <ActivityIndicator size="large" color={colors.primary} />
        </View>
      </SafeAreaView>
    );
  }

  if (error) {
    return (
      <SafeAreaView style={styles.container}>
        <View style={styles.center}>
          <Text style={styles.errorText}>{t('common.error')}</Text>
        </View>
      </SafeAreaView>
    );
  }

  return (
    <SafeAreaView style={styles.container}>
      <FlatList
        data={sections}
        keyExtractor={(item) => item.region}
        renderItem={({item: section}) => (
          <View>
            <Text style={styles.sectionHeader}>{section.region}</Text>
            {section.data.map((server) => (
              <ServerCard
                key={server.id}
                server={server}
                isSelected={selectedServer?.id === server.id}
                isFavorite={favoriteIds.includes(server.id)}
                onPress={() => handleSelectServer(server)}
                onFavoritePress={() => toggleFavorite(server.id)}
              />
            ))}
          </View>
        )}
        contentContainerStyle={[styles.list, contentMaxWidth ? {maxWidth: contentMaxWidth, alignSelf: 'center', width: '100%'} : undefined]}
      />
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  container: {
    flex: 1,
    backgroundColor: colors.background,
  },
  center: {
    flex: 1,
    justifyContent: 'center',
    alignItems: 'center',
  },
  list: {
    paddingVertical: spacing.md,
  },
  sectionHeader: {
    ...typography.captionBold,
    color: colors.textMuted,
    textTransform: 'uppercase',
    letterSpacing: 1,
    paddingHorizontal: spacing.lg,
    paddingTop: spacing.lg,
    paddingBottom: spacing.sm,
  },
  errorText: {
    ...typography.body,
    color: colors.error,
  },
});
