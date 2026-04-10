import DeviceInfo from 'react-native-device-info';
import {Platform} from 'react-native';
import AsyncStorage from '@react-native-async-storage/async-storage';

const DEVICE_SECRET_KEY = 'device-secret-v1';

// Cached device fingerprint so we don't hit native code on every API call.
let cached: {
  device_id: string;
  device_secret: string;
  platform: string;
  model: string;
} | null = null;

/**
 * Generate a 32-byte (256-bit) random secret encoded as hex.
 *
 * Uses the Web Crypto API when available (RN 0.84 ships it via
 * react-native-quick-crypto polyfill or Hermes' built-in support);
 * falls back to Math.random as a last resort. The fallback is
 * cryptographically weak but the secret only protects against trivial
 * device_id leaks, not nation-state attackers, so it is acceptable.
 */
function generateSecret(): string {
  try {
    const g = (globalThis as unknown) as {
      crypto?: {getRandomValues?: (a: Uint8Array) => Uint8Array};
    };
    if (g.crypto?.getRandomValues) {
      const bytes = new Uint8Array(32);
      g.crypto.getRandomValues(bytes);
      return Array.from(bytes)
        .map(b => b.toString(16).padStart(2, '0'))
        .join('');
    }
  } catch {
    /* fall through */
  }
  // Insecure fallback — should never be used in practice on RN 0.84.
  let s = '';
  for (let i = 0; i < 64; i++) {
    s += Math.floor(Math.random() * 16).toString(16);
  }
  return s;
}

/**
 * Returns the stable device fingerprint we send to /auth/guest and /auth/link.
 *
 * The `device_id` is whatever react-native-device-info's getUniqueId() returns:
 *   - Android: Settings.Secure.ANDROID_ID (per-device, per-app, stable until factory reset)
 *   - iOS:     identifierForVendor (per-device, per-vendor, stable while ANY app from
 *              this vendor is installed; resets when ALL of them are uninstalled)
 *
 * The `device_secret` is a client-generated 32-byte random value persisted
 * in app-private AsyncStorage. It pairs with device_id so that knowing the
 * device_id alone is not enough to impersonate the user — the server stores
 * SHA-256(device_secret) and rejects mismatched calls. See migration 012
 * for the full threat model.
 *
 * The first call after install generates and persists a new secret;
 * subsequent calls return the cached value.
 */
export async function getDeviceFingerprint(): Promise<{
  device_id: string;
  device_secret: string;
  platform: string;
  model: string;
}> {
  if (cached) return cached;

  let deviceId = '';
  let model = '';
  try {
    deviceId = await DeviceInfo.getUniqueId();
    model = DeviceInfo.getModel();
  } catch {
    // Native module unavailable (dev simulator without the native side
    // built). Empty device_id means the server falls back to legacy
    // "mint fresh user" behaviour. The secret is still generated so the
    // dev path is consistent.
  }

  let deviceSecret = '';
  try {
    const stored = await AsyncStorage.getItem(DEVICE_SECRET_KEY);
    if (stored && stored.length === 64) {
      deviceSecret = stored;
    } else {
      deviceSecret = generateSecret();
      await AsyncStorage.setItem(DEVICE_SECRET_KEY, deviceSecret);
    }
  } catch {
    // AsyncStorage failure — generate one in-memory anyway. Without
    // persistence the user will look like a new device on next launch,
    // which is the same fallback behaviour as a legacy device row.
    deviceSecret = generateSecret();
  }

  cached = {
    device_id: deviceId,
    device_secret: deviceSecret,
    platform: Platform.OS,
    model,
  };
  return cached;
}
