import { promises as fs } from "fs";
import path from "path";
import {
  ConfigPlugin,
  withAppBuildGradle,
  withDangerousMod,
} from "expo/config-plugins";

/**
 * Let QA builds run against RevenueCat's Test Store (test_* API keys).
 *
 * The SDK refuses a test_ key in a build that was not compiled for debugging: it logs
 * "Test Store API key used in release build", shows a "Wrong API Key" dialog and kills the
 * app on launch. QA is affected because its artifacts are real release builds —
 * `assembleRelease` on Android and a Release-configuration archive on iOS — even though
 * they only ever reach internal testers through Firebase App Distribution.
 *
 * The two platforms decide "is this a debug build?" differently, so each needs its own
 * opt-out:
 *
 *   Android  DefaultIsDebugBuildProvider reads ApplicationInfo.FLAG_DEBUGGABLE at runtime,
 *            so `debuggable true` on the release build type is enough. It only flips the
 *            manifest flag — BuildConfig.DEBUG stays false, so React Native still runs the
 *            bundled JS exactly as before.
 *   iOS      the check is `#if !DEBUG && !BYPASS_SIMULATED_STORE_RELEASE_CHECK` compiled
 *            into the RevenueCat pod, so it can only be disarmed at pod-compile time. The
 *            bypass flag is RevenueCat's own, meant for consumers that ship the SDK
 *            pre-compiled in Release.
 *
 * NEVER apply this to production: app.config.js only adds this plugin when
 * APP_ENV !== 'production', and both pipelines assert the outcome — QA that it landed,
 * production that it did not. A production build carries real appl_/goog_ keys (the
 * pipeline refuses anything else), for which this guard never fires anyway.
 */

const BYPASS_FLAG = "BYPASS_SIMULATED_STORE_RELEASE_CHECK";

const withDebuggableReleaseApk: ConfigPlugin = (config) =>
  withAppBuildGradle(config, (config) => {
    if (config.modResults.language !== "groovy") {
      throw new Error("withRevenueCatTestStore expects a Groovy build.gradle");
    }

    const contents = config.modResults.contents;
    if (contents.includes("debuggable true")) {
      return config;
    }

    // Anchored on the release block inside buildTypes so the debug block, which is already
    // debuggable, is left alone. $2 is the block's indentation, kept so the generated file
    // stays readable when a build fails and someone reads it.
    const patched = contents.replace(
      /(buildTypes\s*\{[\s\S]*?\n(\s*)release\s*\{\n)/,
      "$1$2    debuggable true\n"
    );

    if (patched === contents) {
      throw new Error(
        "withRevenueCatTestStore could not mark the release build type debuggable — the " +
          "prebuild template changed shape. Check android/app/build.gradle after prebuild."
      );
    }

    config.modResults.contents = patched;
    return config;
  });

const withRevenueCatBypassPodFlag: ConfigPlugin = (config) =>
  withDangerousMod(config, [
    "ios",
    async (config) => {
      const podfilePath = path.join(
        config.modRequest.platformProjectRoot,
        "Podfile"
      );
      const contents = await fs.readFile(podfilePath, "utf8");

      if (contents.includes(BYPASS_FLAG)) {
        return config;
      }

      // Appended after react_native_post_install rather than before it: that helper sets
      // SWIFT_ACTIVE_COMPILATION_CONDITIONS across every target for the Debug config, and
      // would overwrite ours if it ran second.
      const snippet = `
    # Added by plugins/withRevenueCatTestStore (QA/dev only — never on APP_ENV=production).
    # Disarms the SDK's "Test Store key in a Release build" guard, which otherwise kills the
    # app on launch because QA archives with -configuration Release.
    installer.pods_project.targets.each do |rc_target|
      next unless rc_target.name == 'RevenueCat'
      rc_target.build_configurations.each do |rc_config|
        conditions = rc_config.build_settings['SWIFT_ACTIVE_COMPILATION_CONDITIONS'] || '$(inherited)'
        conditions = conditions.join(' ') if conditions.is_a?(Array)
        next if conditions.include?('${BYPASS_FLAG}')
        rc_config.build_settings['SWIFT_ACTIVE_COMPILATION_CONDITIONS'] = "#{conditions} ${BYPASS_FLAG}"
      end
    end
`;

      const patched = contents.replace(
        /(post_install do \|installer\|\n[\s\S]*?react_native_post_install\([\s\S]*?\n\s*\)\n)/,
        `$1${snippet}`
      );

      if (patched === contents) {
        throw new Error(
          "withRevenueCatTestStore could not extend the Podfile post_install hook — the " +
            "prebuild template changed shape. Check ios/Podfile after prebuild."
        );
      }

      await fs.writeFile(podfilePath, patched);
      return config;
    },
  ]);

const withRevenueCatTestStore: ConfigPlugin = (config) =>
  withRevenueCatBypassPodFlag(withDebuggableReleaseApk(config));

export default withRevenueCatTestStore;
