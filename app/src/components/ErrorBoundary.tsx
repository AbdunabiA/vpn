import React from 'react';
import {View, Text, StyleSheet, TouchableOpacity, SafeAreaView} from 'react-native';
import {useTranslation} from 'react-i18next';
import {colors, typography, spacing, borderRadius} from '../theme';

// ---- Fallback UI (functional — can use hooks) ----

interface FallbackProps {
  errorMessage: string | null;
  onRetry: () => void;
}

function ErrorFallback({errorMessage, onRetry}: FallbackProps) {
  const {t} = useTranslation();

  return (
    <SafeAreaView style={styles.container}>
      <View style={styles.card}>
        <Text style={styles.icon}>⚠</Text>
        <Text style={styles.title}>{t('errorBoundary.title')}</Text>
        <Text style={styles.message}>{t('errorBoundary.message')}</Text>
        {__DEV__ && errorMessage ? (
          <Text style={styles.devMessage}>{errorMessage}</Text>
        ) : null}
        <TouchableOpacity
          style={styles.retryButton}
          onPress={onRetry}
          activeOpacity={0.8}
          accessibilityRole="button"
          accessibilityLabel={t('errorBoundary.retry')}>
          <Text style={styles.retryText}>{t('errorBoundary.retry')}</Text>
        </TouchableOpacity>
      </View>
    </SafeAreaView>
  );
}

// ---- Class boundary (required for componentDidCatch) ----

interface ErrorBoundaryProps {
  children: React.ReactNode;
}

interface ErrorBoundaryState {
  hasError: boolean;
  errorMessage: string | null;
}

export class ErrorBoundary extends React.Component<ErrorBoundaryProps, ErrorBoundaryState> {
  constructor(props: ErrorBoundaryProps) {
    super(props);
    this.state = {hasError: false, errorMessage: null};
  }

  static getDerivedStateFromError(error: Error): ErrorBoundaryState {
    return {hasError: true, errorMessage: error.message};
  }

  componentDidCatch(error: Error, info: React.ErrorInfo): void {
    console.error('[ErrorBoundary] Caught error:', error, info.componentStack);
  }

  handleRetry = (): void => {
    this.setState({hasError: false, errorMessage: null});
  };

  render() {
    if (this.state.hasError) {
      return (
        <ErrorFallback
          errorMessage={this.state.errorMessage}
          onRetry={this.handleRetry}
        />
      );
    }

    return this.props.children;
  }
}

// ---- Styles ----

const styles = StyleSheet.create({
  container: {
    flex: 1,
    backgroundColor: colors.background,
    justifyContent: 'center',
    alignItems: 'center',
    paddingHorizontal: spacing.lg,
  },
  card: {
    backgroundColor: colors.surface,
    borderRadius: borderRadius.lg,
    padding: spacing.xl,
    borderWidth: 1,
    borderColor: colors.border,
    alignItems: 'center',
    width: '100%',
  },
  icon: {
    fontSize: 48,
    marginBottom: spacing.md,
  },
  title: {
    ...typography.h2,
    color: colors.textPrimary,
    textAlign: 'center',
    marginBottom: spacing.sm,
  },
  message: {
    ...typography.body,
    color: colors.textSecondary,
    textAlign: 'center',
    marginBottom: spacing.lg,
  },
  devMessage: {
    ...typography.caption,
    color: colors.error,
    textAlign: 'center',
    marginBottom: spacing.lg,
    fontFamily: 'monospace',
  },
  retryButton: {
    backgroundColor: colors.primary,
    borderRadius: borderRadius.sm,
    paddingVertical: spacing.sm,
    paddingHorizontal: spacing.xl,
    alignItems: 'center',
  },
  retryText: {
    ...typography.bodyBold,
    color: colors.textPrimary,
  },
});
