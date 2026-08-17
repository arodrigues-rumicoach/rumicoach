import { promises as fs } from "fs";
import path from "path";
import {
  ConfigPlugin,
  withAndroidManifest,
  withDangerousMod,
  AndroidConfig,
} from "expo/config-plugins";

// Trusts user-added CAs (Charles/mitmproxy/Proxyman) so HTTPS traffic can be
// intercepted on QA/dev builds. QA APKs are *release* builds, so the trust must
// live in <base-config> — <debug-overrides> alone only covers debuggable builds.
// NEVER apply this to production: app.config.js only adds this plugin when
// APP_ENV !== 'production'.
const NETWORK_SECURITY_CONFIG_XML = `<?xml version="1.0" encoding="utf-8"?>
<network-security-config>
  <debug-overrides>
    <trust-anchors>
      <!-- Trust user added CAs while debuggable only -->
      <certificates src="user" />
      <certificates src="system" />
    </trust-anchors>
  </debug-overrides>

  <base-config cleartextTrafficPermitted="true">
    <trust-anchors>
      <certificates src="system" />
      <certificates src="user" />
    </trust-anchors>
  </base-config>
</network-security-config>
`;

const withNetworkSecurityConfig: ConfigPlugin = (config) => {
  config = withDangerousMod(config, [
    "android",
    async (config) => {
      const xmlDir = path.join(
        config.modRequest.platformProjectRoot,
        "app",
        "src",
        "main",
        "res",
        "xml"
      );
      await fs.mkdir(xmlDir, { recursive: true });
      await fs.writeFile(
        path.join(xmlDir, "network_security_config.xml"),
        NETWORK_SECURITY_CONFIG_XML
      );
      return config;
    },
  ]);

  config = withAndroidManifest(config, (config) => {
    const application = AndroidConfig.Manifest.getMainApplicationOrThrow(
      config.modResults
    );
    application.$["android:networkSecurityConfig"] =
      "@xml/network_security_config";
    return config;
  });

  return config;
};

export default withNetworkSecurityConfig;
