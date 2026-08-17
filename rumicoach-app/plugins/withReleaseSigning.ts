import { ConfigPlugin, withAppBuildGradle } from "expo/config-plugins";

/**
 * Point the Android release build at the real upload keystore.
 *
 * Expo's prebuild template signs BOTH debug and release with the debug keystore. Firebase
 * App Distribution accepts that, so it went unnoticed — but Google Play rejects a
 * debug-signed artifact outright, and android/ is gitignored and regenerated on every CI
 * run, so the fix cannot live in build.gradle.
 *
 * The keystore is read from the environment rather than gradle.properties so no password is
 * ever written to disk. When RUMI_UPLOAD_STORE_FILE is unset this plugin does nothing, which
 * keeps local builds and the QA pipeline on debug signing exactly as before.
 */
const withReleaseSigning: ConfigPlugin = (config) =>
  withAppBuildGradle(config, (config) => {
    if (config.modResults.language !== "groovy") {
      throw new Error("withReleaseSigning expects a Groovy build.gradle");
    }
    if (!process.env.RUMI_UPLOAD_STORE_FILE) {
      return config;
    }

    let contents = config.modResults.contents;

    if (contents.includes("signingConfigs.release")) {
      return config;
    }

    // Add a release signingConfig alongside the template's debug one.
    // prod-upload.keystore is PKCS12 (header 3082…, not JKS's feedfeed). PKCS12 protects the
    // key with the *store* password and has no separate key password, so falling back to the
    // store password is what makes signing work — passing a different value fails with
    // "Get Key failed: Given final block not properly padded", which is what broke #156.
    // The fallback is kept rather than hardcoded so a future JKS keystore still works.
    contents = contents.replace(
      /(signingConfigs\s*\{)/,
      `$1
        release {
            storeFile file(System.getenv("RUMI_UPLOAD_STORE_FILE"))
            storePassword System.getenv("RUMI_UPLOAD_STORE_PASSWORD")
            keyAlias System.getenv("RUMI_UPLOAD_KEY_ALIAS")
            keyPassword System.getenv("RUMI_UPLOAD_KEY_PASSWORD") ?: System.getenv("RUMI_UPLOAD_STORE_PASSWORD")
        }`
    );

    // Repoint only the release build type. The debug block keeps debug signing, so the
    // replace is anchored on the release block rather than done globally.
    contents = contents.replace(
      /(buildTypes\s*\{[\s\S]*?release\s*\{[\s\S]*?)signingConfig signingConfigs\.debug/,
      "$1signingConfig signingConfigs.release"
    );

    if (!contents.includes("signingConfig signingConfigs.release")) {
      throw new Error(
        "withReleaseSigning could not rewrite the release signingConfig — the prebuild " +
          "template changed shape. Check android/app/build.gradle after prebuild."
      );
    }

    config.modResults.contents = contents;
    return config;
  });

export default withReleaseSigning;
