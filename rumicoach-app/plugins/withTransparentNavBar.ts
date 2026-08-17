import {
  ConfigPlugin,
  withAndroidStyles,
  withGradleProperties,
  withMainActivity,
  AndroidConfig,
} from "expo/config-plugins";

const withTransparentNavBar: ConfigPlugin = (config) => {
  config = withAndroidStyles(config, (config) => {
    const styles = config.modResults;

    const appTheme = AndroidConfig.Styles.getAppThemeGroup();

    AndroidConfig.Styles.assignStylesValue(styles, {
      add: true,
      name: "android:statusBarColor",
      value: "@android:color/transparent",
      parent: appTheme,
    });

    AndroidConfig.Styles.assignStylesValue(styles, {
      add: true,
      name: "android:navigationBarColor",
      value: "@android:color/transparent",
      parent: appTheme,
    });

    AndroidConfig.Styles.assignStylesValue(styles, {
      add: true,
      name: "android:windowBackground",
      value: "@color/splashscreen_background",
      parent: appTheme,
    });

    AndroidConfig.Styles.assignStylesValue(styles, {
      add: true,
      name: "android:enforceNavigationBarContrast",
      value: "false",
      targetApi: "29",
      parent: appTheme,
    });

    return config;
  });

  config = withGradleProperties(config, (config) => {
    const props = config.modResults;

    const upsert = (key: string, value: string) => {
      const existing = props.find(
        (p): p is { type: "property"; key: string; value: string } =>
          p.type === "property" && p.key === key
      );
      if (existing) {
        existing.value = value;
      } else {
        props.push({ type: "property", key, value });
      }
    };

    upsert("edgeToEdgeEnabled", "true");
    upsert("expo.edgeToEdgeEnabled", "true");

    return config;
  });

  config = withMainActivity(config, (config) => {
    if (config.modResults.language !== "kt") return config;

    let contents = config.modResults.contents;

    if (contents.includes("WindowCompat.setDecorFitsSystemWindows")) {
      return config;
    }

    if (!contents.includes("import androidx.core.view.WindowCompat")) {
      contents = contents.replace(
        /(import expo\.modules\.\S+)/,
        "$1\nimport androidx.core.view.WindowCompat\nimport androidx.core.view.WindowInsetsControllerCompat"
      );
    }

    contents = contents.replace(
      /(super\.onCreate\(null\))/,
      `$1

    WindowCompat.setDecorFitsSystemWindows(window, false)
    window.navigationBarColor = android.graphics.Color.TRANSPARENT
    window.statusBarColor = android.graphics.Color.TRANSPARENT
    WindowInsetsControllerCompat(window, window.decorView).apply {
      hide(android.view.WindowInsets.Type.navigationBars())
      systemBarsBehavior = WindowInsetsControllerCompat.BEHAVIOR_SHOW_TRANSIENT_BARS_BY_SWIPE
    }`
    );

    config.modResults.contents = contents;
    return config;
  });

  return config;
};

export default withTransparentNavBar;
