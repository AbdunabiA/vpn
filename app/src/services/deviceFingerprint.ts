import DeviceInfo from 'react-native-device-info';
import {Platform} from 'react-native';

// Cached device fingerprint so we don't hit native code on every API call.
let cached: {device_id: string; platform: string; model: string} | null = null;

/**
 * Returns the stable device fingerprint we send to /auth/guest and /auth/link.
 *
 * The `device_id` is whatever react-native-device-info's getUniqueId() returns:
 *   - Android: Settings.Secure.ANDROID_ID (per-device, per-app, stable until factory reset)
 *   - iOS:     identifierForVendor (per-device, per-vendor, stable while ANY app from
 *              this vendor is installed; resets when ALL of them are uninstalled)
 *
 * Both are privacy-friendly (not user-trackable across apps/vendors) but
 * stable enough to defeat trivial reinstall abuse and to support the
 * share-code linking flow.
 */
export async function getDeviceFingerprint(): Promise<{
  device_id: string;
  platform: string;
  model: string;
}> {
  if (cached) return cached;
  try {
    const deviceId = await DeviceInfo.getUniqueId();
    const model = DeviceInfo.getModel();
    cached = {
      device_id: deviceId,
      platform: Platform.OS,
      model,
    };
  } catch {
    // Fallback when native module is unavailable (e.g. dev simulator without
    // the native side built). Empty device_id means the server falls back to
    // legacy "always mint a new user" behaviour, which is fine for dev.
    cached = {device_id: '', platform: Platform.OS, model: ''};
  }
  return cached;
}
