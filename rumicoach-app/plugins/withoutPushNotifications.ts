import { ConfigPlugin, withDangerousMod } from "expo/config-plugins";
import fs from "fs";
import path from "path";

function findEntitlementsFile(dir: string): string | null {
  const entries = fs.readdirSync(dir, { withFileTypes: true });
  for (const entry of entries) {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory() && entry.name !== "Pods") {
      const found = findEntitlementsFile(full);
      if (found) return found;
    } else if (entry.isFile() && entry.name.endsWith(".entitlements")) {
      return full;
    }
  }
  return null;
}

const withoutPushNotifications: ConfigPlugin = (config) => {
  config = withDangerousMod(config, [
    "ios",
    (config) => {
      const root = config.modRequest.platformProjectRoot;
      const entitlementsPath = findEntitlementsFile(root);

      if (entitlementsPath) {
        let contents = fs.readFileSync(entitlementsPath, "utf-8");
        contents = contents.replace(
          /<key>aps-environment<\/key>\s*<string>[^<]*<\/string>/,
          ""
        );
        fs.writeFileSync(entitlementsPath, contents);
      }

      return config;
    },
  ]);

  return config;
};

export default withoutPushNotifications;
