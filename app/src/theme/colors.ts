// App color palette — dark theme focused (common for VPN apps)

export const colors = {
  // Backgrounds
  background: '#0A0E17',
  surface: '#141926',
  surfaceLight: '#1E2538',
  card: '#1A2032',

  // Primary — electric blue (trust, security)
  primary: '#3B82F6',
  primaryLight: '#60A5FA',
  primaryDark: '#2563EB',

  // Accent — green for connected state
  success: '#22C55E',
  successLight: '#4ADE80',
  successDark: '#16A34A',

  // Status colors
  warning: '#F59E0B',
  error: '#EF4444',
  errorLight: '#FCA5A5',

  // Text
  textPrimary: '#F8FAFC',
  textSecondary: '#94A3B8',
  textMuted: '#64748B',

  // Borders
  border: '#1E293B',
  borderLight: '#334155',

  // Transparent overlays
  overlay: 'rgba(0, 0, 0, 0.5)',
  shimmer: 'rgba(255, 255, 255, 0.05)',
} as const;

// Light theme variant (for future use)
export const lightColors = {
  background: '#F8FAFC',
  surface: '#FFFFFF',
  surfaceLight: '#F1F5F9',
  card: '#FFFFFF',
  primary: '#2563EB',
  primaryLight: '#3B82F6',
  primaryDark: '#1D4ED8',
  success: '#16A34A',
  successLight: '#22C55E',
  successDark: '#15803D',
  warning: '#D97706',
  error: '#DC2626',
  errorLight: '#EF4444',
  textPrimary: '#0F172A',
  textSecondary: '#475569',
  textMuted: '#94A3B8',
  border: '#E2E8F0',
  borderLight: '#CBD5E1',
  overlay: 'rgba(0, 0, 0, 0.3)',
  shimmer: 'rgba(0, 0, 0, 0.03)',
} as const;
