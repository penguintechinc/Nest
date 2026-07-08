/**
 * License client - handles license validation and feature flags
 * Gracefully degrades on network errors; never throws on missing license
 */

let cachedLicenseInfo = null;
let cachedFeatures = null;
const CACHE_TTL = 60 * 60 * 1000; // 1 hour
let lastCacheTime = 0;

const LICENSE_SERVER = process.env.VITE_LICENSE_SERVER || 'https://license.penguintech.io';
const LICENSE_KEY = process.env.VITE_LICENSE_KEY || 'no-license';

/**
 * Initialize licensing - fetch and cache license info
 * Never throws; returns null if license server unavailable
 */
export async function initializeLicensing() {
  try {
    const response = await fetch(`${LICENSE_SERVER}/validate`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ license_key: LICENSE_KEY }),
      timeout: 5000,
    });

    if (!response.ok) {
      console.warn('[license-client] License validation returned non-200 status');
      return getDefaultLicense();
    }

    const data = await response.json();
    cachedLicenseInfo = data;
    lastCacheTime = Date.now();
    return data;
  } catch (error) {
    console.warn('[license-client] License server unreachable - using defaults', { error: error.message });
    return getDefaultLicense();
  }
}

/**
 * Get license client instance
 * Returns an object with feature checking methods
 */
export function getClient() {
  return {
    /**
     * Check if a feature is available
     * Never throws; returns false if unavailable
     */
    async checkFeature(featureName) {
      try {
        if (!cachedFeatures) {
          cachedFeatures = await getAllFeatures();
        }
        const feature = cachedFeatures.find(f => f.name === featureName);
        return feature?.enabled ?? false;
      } catch (error) {
        console.warn(`[license-client] Feature check failed for ${featureName}`);
        return false;
      }
    },

    /**
     * Get all available features
     * Never throws; returns empty array if unavailable
     */
    async getAllFeatures() {
      try {
        return await getAllFeatures();
      } catch (error) {
        console.warn('[license-client] Failed to fetch features');
        return [];
      }
    },

    /**
     * Send keepalive ping with feature usage
     * Fire-and-forget; never throws
     */
    keepalive(data) {
      return fetch(`${LICENSE_SERVER}/keepalive`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ license_key: LICENSE_KEY, ...data }),
      }).catch(error => {
        console.warn('[license-client] Keepalive failed', { error: error.message });
      });
    },
  };
}

/**
 * Get all available features
 * Never throws; returns defaults if unavailable
 */
async function getAllFeatures() {
  if (cachedFeatures && Date.now() - lastCacheTime < CACHE_TTL) {
    return cachedFeatures;
  }

  try {
    const response = await fetch(`${LICENSE_SERVER}/features`, {
      headers: { 'X-License-Key': LICENSE_KEY },
      timeout: 5000,
    });

    if (!response.ok) {
      return getDefaultFeatures();
    }

    cachedFeatures = await response.json();
    lastCacheTime = Date.now();
    return cachedFeatures;
  } catch (error) {
    console.warn('[license-client] Failed to fetch features - using defaults');
    return getDefaultFeatures();
  }
}

/**
 * Default license info when server unavailable
 * Disables all enterprise features, allows basic usage
 */
function getDefaultLicense() {
  return {
    customer: 'Trial',
    tier: 'free',
    features: [],
    expires_at: new Date(Date.now() + 30 * 24 * 60 * 60 * 1000).toISOString(),
  };
}

/**
 * Default features when server unavailable
 * Only enable safe, non-enterprise features
 */
function getDefaultFeatures() {
  return [
    { name: 'basic_operations', enabled: true },
    { name: 'read_operations', enabled: true },
    { name: 'advanced_analytics', enabled: false },
    { name: 'enterprise_features', enabled: false },
    { name: 'user_management', enabled: false },
    { name: 'enterprise_reports', enabled: false },
  ];
}
