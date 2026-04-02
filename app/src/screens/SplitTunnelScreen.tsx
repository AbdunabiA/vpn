import React, {useEffect, useState, useCallback} from 'react';
import {
  View,
  Text,
  Switch,
  StyleSheet,
  SafeAreaView,
  FlatList,
  TextInput,
  TouchableOpacity,
  ActivityIndicator,
  Platform,
  KeyboardAvoidingView,
} from 'react-native';
import {useTranslation} from 'react-i18next';
import {useSettingsStore} from '../stores/settingsStore';
import {getInstalledApps} from '../services/vpnBridge';
import type {InstalledApp} from '../services/vpnBridge';
import {colors, typography, spacing, borderRadius} from '../theme';

// ---------------------------------------------------------------------------
// Android: per-app split tunneling
// ---------------------------------------------------------------------------

function AppItem({
  app,
  excluded,
  onToggle,
}: {
  app: InstalledApp;
  excluded: boolean;
  onToggle: (pkg: string, value: boolean) => void;
}) {
  return (
    <View style={styles.appRow}>
      <View style={styles.appIconPlaceholder}>
        <Text style={styles.appIconLetter}>
          {app.appName.charAt(0).toUpperCase()}
        </Text>
      </View>
      <View style={styles.appInfo}>
        <Text style={styles.appName} numberOfLines={1}>
          {app.appName}
        </Text>
        <Text style={styles.appPackage} numberOfLines={1}>
          {app.packageName}
        </Text>
      </View>
      <Switch
        value={excluded}
        onValueChange={(v) => onToggle(app.packageName, v)}
        trackColor={{true: colors.primary, false: colors.surfaceLight}}
        thumbColor={colors.textPrimary}
      />
    </View>
  );
}

function AndroidSplitTunnel() {
  const {t} = useTranslation();
  const {excludedApps, setExcludedApps} = useSettingsStore();

  const [apps, setApps] = useState<InstalledApp[]>([]);
  const [filtered, setFiltered] = useState<InstalledApp[]>([]);
  const [search, setSearch] = useState('');
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    getInstalledApps()
      .then((result) => {
        // Sort user apps first, then system apps; within each group sort by name
        const sorted = [...result].sort((a, b) => {
          if (a.isSystemApp !== b.isSystemApp) {
            return a.isSystemApp ? 1 : -1;
          }
          return a.appName.localeCompare(b.appName);
        });
        setApps(sorted);
        setFiltered(sorted);
      })
      .catch((err) => {
        setError(String(err?.message ?? err));
      })
      .finally(() => setLoading(false));
  }, []);

  const onSearch = useCallback(
    (text: string) => {
      setSearch(text);
      const lower = text.toLowerCase();
      setFiltered(
        apps.filter(
          (a) =>
            a.appName.toLowerCase().includes(lower) ||
            a.packageName.toLowerCase().includes(lower),
        ),
      );
    },
    [apps],
  );

  const onToggle = useCallback(
    (pkg: string, value: boolean) => {
      const next = value
        ? [...excludedApps, pkg]
        : excludedApps.filter((p) => p !== pkg);
      setExcludedApps(next);
    },
    [excludedApps, setExcludedApps],
  );

  if (loading) {
    return (
      <View style={styles.centered}>
        <ActivityIndicator color={colors.primary} size="large" />
        <Text style={styles.loadingText}>{t('common.loading')}</Text>
      </View>
    );
  }

  if (error) {
    return (
      <View style={styles.centered}>
        <Text style={styles.errorText}>{t('common.error')}: {error}</Text>
      </View>
    );
  }

  if (apps.length === 0) {
    return (
      <View style={styles.centered}>
        <Text style={styles.emptyText}>{t('splitTunnel.noApps')}</Text>
      </View>
    );
  }

  const excludedSet = new Set(excludedApps);
  const excludedCount = excludedApps.length;

  return (
    <View style={styles.flex}>
      {/* Search bar */}
      <View style={styles.searchWrapper}>
        <TextInput
          style={styles.searchInput}
          value={search}
          onChangeText={onSearch}
          placeholder={t('splitTunnel.searchApps')}
          placeholderTextColor={colors.textMuted}
          clearButtonMode="while-editing"
          autoCapitalize="none"
          autoCorrect={false}
        />
      </View>

      {/* Excluded count badge */}
      {excludedCount > 0 && (
        <View style={styles.badgeRow}>
          <Text style={styles.badgeText}>
            {t('splitTunnel.excludedApps')}: {excludedCount}
          </Text>
        </View>
      )}

      <FlatList
        data={filtered}
        keyExtractor={(item) => item.packageName}
        renderItem={({item}) => (
          <AppItem
            app={item}
            excluded={excludedSet.has(item.packageName)}
            onToggle={onToggle}
          />
        )}
        ItemSeparatorComponent={() => <View style={styles.separator} />}
        contentContainerStyle={styles.listContent}
        keyboardShouldPersistTaps="handled"
      />
    </View>
  );
}

// ---------------------------------------------------------------------------
// iOS: domain-based split tunneling
// ---------------------------------------------------------------------------

function IosSplitTunnel() {
  const {t} = useTranslation();
  const {excludedDomains, setExcludedDomains} = useSettingsStore();
  const [input, setInput] = useState('');

  const addDomain = useCallback(() => {
    const domain = input.trim().toLowerCase();
    if (!domain) return;
    if (excludedDomains.includes(domain)) {
      setInput('');
      return;
    }
    setExcludedDomains([...excludedDomains, domain]);
    setInput('');
  }, [input, excludedDomains, setExcludedDomains]);

  const removeDomain = useCallback(
    (domain: string) => {
      setExcludedDomains(excludedDomains.filter((d) => d !== domain));
    },
    [excludedDomains, setExcludedDomains],
  );

  return (
    <KeyboardAvoidingView style={styles.flex} behavior="padding">
      <View style={styles.iosContent}>
        {/* Input row */}
        <View style={styles.domainInputRow}>
          <TextInput
            style={styles.domainInput}
            value={input}
            onChangeText={setInput}
            placeholder={t('splitTunnel.domainPlaceholder')}
            placeholderTextColor={colors.textMuted}
            autoCapitalize="none"
            autoCorrect={false}
            keyboardType="url"
            onSubmitEditing={addDomain}
            returnKeyType="done"
          />
          <TouchableOpacity
            style={[
              styles.addButton,
              !input.trim() && styles.addButtonDisabled,
            ]}
            onPress={addDomain}
            disabled={!input.trim()}>
            <Text style={styles.addButtonText}>{t('splitTunnel.addDomain')}</Text>
          </TouchableOpacity>
        </View>

        {/* Section label */}
        {excludedDomains.length > 0 && (
          <Text style={styles.sectionLabel}>
            {t('splitTunnel.domains')} ({excludedDomains.length})
          </Text>
        )}

        {/* Domain list */}
        {excludedDomains.length === 0 ? (
          <View style={styles.emptyDomains}>
            <Text style={styles.emptyText}>{t('splitTunnel.noApps')}</Text>
          </View>
        ) : (
          <View style={styles.card}>
            {excludedDomains.map((domain, index) => (
              <View key={domain}>
                <View style={styles.domainRow}>
                  <Text style={styles.domainText}>{domain}</Text>
                  <TouchableOpacity
                    onPress={() => removeDomain(domain)}
                    hitSlop={{top: 8, bottom: 8, left: 8, right: 8}}>
                    <Text style={styles.removeButton}>✕</Text>
                  </TouchableOpacity>
                </View>
                {index < excludedDomains.length - 1 && (
                  <View style={styles.separator} />
                )}
              </View>
            ))}
          </View>
        )}
      </View>
    </KeyboardAvoidingView>
  );
}

// ---------------------------------------------------------------------------
// Screen entry point
// ---------------------------------------------------------------------------

export function SplitTunnelScreen() {
  const {t} = useTranslation();

  return (
    <SafeAreaView style={styles.container}>
      {/* Description banner */}
      <View style={styles.descriptionBanner}>
        <Text style={styles.descriptionText}>
          {t('splitTunnel.description')}
        </Text>
      </View>

      {Platform.OS === 'android' ? (
        <AndroidSplitTunnel />
      ) : (
        <IosSplitTunnel />
      )}
    </SafeAreaView>
  );
}

// ---------------------------------------------------------------------------
// Styles
// ---------------------------------------------------------------------------

const styles = StyleSheet.create({
  flex: {
    flex: 1,
  },
  container: {
    flex: 1,
    backgroundColor: colors.background,
  },
  centered: {
    flex: 1,
    alignItems: 'center',
    justifyContent: 'center',
    padding: spacing.lg,
  },
  loadingText: {
    ...typography.caption,
    color: colors.textMuted,
    marginTop: spacing.sm,
  },
  errorText: {
    ...typography.caption,
    color: colors.error,
    textAlign: 'center',
  },
  emptyText: {
    ...typography.body,
    color: colors.textMuted,
    textAlign: 'center',
  },
  descriptionBanner: {
    backgroundColor: colors.surface,
    borderBottomWidth: 1,
    borderBottomColor: colors.border,
    paddingHorizontal: spacing.md,
    paddingVertical: spacing.sm,
  },
  descriptionText: {
    ...typography.caption,
    color: colors.textSecondary,
  },
  // Search
  searchWrapper: {
    paddingHorizontal: spacing.md,
    paddingVertical: spacing.sm,
  },
  searchInput: {
    backgroundColor: colors.surface,
    borderWidth: 1,
    borderColor: colors.border,
    borderRadius: borderRadius.md,
    paddingHorizontal: spacing.md,
    paddingVertical: spacing.sm,
    ...typography.body,
    color: colors.textPrimary,
  },
  badgeRow: {
    paddingHorizontal: spacing.md,
    paddingBottom: spacing.xs,
  },
  badgeText: {
    ...typography.captionBold,
    color: colors.primary,
  },
  listContent: {
    paddingBottom: spacing.lg,
  },
  // App row
  appRow: {
    flexDirection: 'row',
    alignItems: 'center',
    paddingHorizontal: spacing.md,
    paddingVertical: spacing.sm,
    backgroundColor: colors.background,
  },
  appIconPlaceholder: {
    width: 40,
    height: 40,
    borderRadius: borderRadius.sm,
    backgroundColor: colors.surfaceLight,
    alignItems: 'center',
    justifyContent: 'center',
    marginRight: spacing.sm,
  },
  appIconLetter: {
    ...typography.bodyBold,
    color: colors.textSecondary,
  },
  appInfo: {
    flex: 1,
    marginRight: spacing.sm,
  },
  appName: {
    ...typography.body,
    color: colors.textPrimary,
  },
  appPackage: {
    ...typography.caption,
    color: colors.textMuted,
  },
  separator: {
    height: 1,
    backgroundColor: colors.border,
    marginLeft: spacing.md,
  },
  // iOS domain input
  iosContent: {
    flex: 1,
    padding: spacing.md,
  },
  domainInputRow: {
    flexDirection: 'row',
    alignItems: 'center',
    marginBottom: spacing.md,
    gap: spacing.sm,
  },
  domainInput: {
    flex: 1,
    backgroundColor: colors.surface,
    borderWidth: 1,
    borderColor: colors.border,
    borderRadius: borderRadius.md,
    paddingHorizontal: spacing.md,
    paddingVertical: spacing.sm,
    ...typography.body,
    color: colors.textPrimary,
  },
  addButton: {
    backgroundColor: colors.primary,
    borderRadius: borderRadius.md,
    paddingHorizontal: spacing.md,
    paddingVertical: spacing.sm,
  },
  addButtonDisabled: {
    backgroundColor: colors.surfaceLight,
  },
  addButtonText: {
    ...typography.captionBold,
    color: colors.textPrimary,
  },
  sectionLabel: {
    ...typography.captionBold,
    color: colors.textMuted,
    textTransform: 'uppercase',
    letterSpacing: 1,
    marginBottom: spacing.sm,
    marginLeft: spacing.xs,
  },
  card: {
    backgroundColor: colors.surface,
    borderRadius: borderRadius.md,
    borderWidth: 1,
    borderColor: colors.border,
    overflow: 'hidden',
  },
  domainRow: {
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'space-between',
    paddingHorizontal: spacing.md,
    paddingVertical: spacing.md,
  },
  domainText: {
    ...typography.body,
    color: colors.textPrimary,
    flex: 1,
  },
  removeButton: {
    ...typography.body,
    color: colors.textMuted,
    marginLeft: spacing.sm,
  },
  emptyDomains: {
    marginTop: spacing.xl,
    alignItems: 'center',
  },
});
