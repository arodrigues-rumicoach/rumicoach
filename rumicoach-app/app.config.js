// Dynamic Expo config, merged on top of app.json (app.json stays the base config;
// everything returned here overrides it).
//
// Select the target environment with APP_ENV — set per EAS build profile in eas.json:
//   APP_ENV=qa          → coach.rumi.app.qa + QA Firebase files (default)
//   APP_ENV=production  → coach.rumi.app    + prod Firebase files
//
// The prod Firebase files (GoogleService-Info.prod.plist, plugins/google-services.prod.json)
// must be downloaded from the PROD Firebase project before a production build will prebuild.
const APP_ENV = process.env.APP_ENV || 'qa';
const isProd = APP_ENV === 'production';

// Google Sign-In iOS URL scheme = the reversed iOS OAuth client ID. Derived from the
// same env var the sign-in code reads (src/config.ts), so it always matches the
// active Firebase project instead of being hardcoded.
const iosClientId = process.env.EXPO_PUBLIC_GOOGLE_CLIENT_ID_IOS || '';
const googleScheme = iosClientId
  ? `com.googleusercontent.apps.${iosClientId.replace('.apps.googleusercontent.com', '')}`
  : null;

// Unique build number per CI build (Bitbucket exports BUILD_NUMBER from
// $BITBUCKET_BUILD_NUMBER), so App Distribution shows "1.0.0 (123)" instead of
// every build being "1.0.0 (1)". Local builds without it keep the defaults.
const buildNumber = parseInt(process.env.BUILD_NUMBER || '', 10);
const hasBuildNumber = Number.isFinite(buildNumber) && buildNumber > 0;

module.exports = ({ config }) => ({
  ...config,
  name: isProd ? 'Rumi' : 'Rumi QA',
  // QA/dev-only plugins, never added on production:
  //   withNetworkSecurityConfig  trusts user-added CAs so HTTPS can be proxied through
  //                              Charles/mitmproxy.
  //   withRevenueCatTestStore    lets the RevenueCat Test Store key (test_*) run in QA's
  //                              release builds instead of killing the app on launch.
  plugins: isProd
    ? config.plugins
    : [
        ...config.plugins,
        './plugins/withNetworkSecurityConfig',
        './plugins/withRevenueCatTestStore',
      ],
  scheme: ['rumi', ...(googleScheme ? [googleScheme] : [])],
  ios: {
    ...config.ios,
    bundleIdentifier: isProd ? 'coach.rumi.app' : 'coach.rumi.app.qa',
    googleServicesFile: isProd
      ? './GoogleService-Info.prod.plist'
      : './GoogleService-Info.plist',
    ...(hasBuildNumber ? { buildNumber: String(buildNumber) } : {}),
  },
  android: {
    ...config.android,
    package: isProd ? 'coach.rumi.app' : 'coach.rumi.app.qa',
    googleServicesFile: isProd
      ? './plugins/google-services.prod.json'
      : './plugins/google-services.json',
    ...(hasBuildNumber ? { versionCode: buildNumber } : {}),
  },
});
