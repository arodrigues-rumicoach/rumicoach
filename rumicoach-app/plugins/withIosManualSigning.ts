import { ConfigPlugin, withXcodeProject } from "expo/config-plugins";

/**
 * Put the iOS app target on manual signing with an explicit provisioning profile.
 *
 * Expo's prebuild template leaves the project on automatic signing, which has no profile to
 * fall back on in CI. The obvious fix — passing CODE_SIGN_STYLE and
 * PROVISIONING_PROFILE_SPECIFIER on the xcodebuild command line — does not work: command-line
 * build settings have the highest precedence in Xcode and apply to EVERY target in the
 * workspace, including the ~200 Pods targets. Those are static libraries that cannot carry a
 * profile, and since Xcode 14 that is an error rather than a warning:
 *
 *   error: EXApplication does not support provisioning profiles, but provisioning profile
 *   Rumi QA Ad Hoc has been manually specified.
 *
 * That failed #176/#177. There is no way to scope a command-line setting to one target, so
 * the settings have to be written into the project instead — which is what this does, to the
 * app target's Release configuration only. ios/ is gitignored and regenerated on every CI
 * run, so it cannot live in the checked-in project either.
 *
 * Driven by the environment, like withReleaseSigning: with RUMI_IOS_PROFILE_SPECIFIER unset
 * this plugin does nothing and local builds keep automatic signing exactly as before.
 */
const withIosManualSigning: ConfigPlugin = (config) =>
  withXcodeProject(config, (config) => {
    const specifier = process.env.RUMI_IOS_PROFILE_SPECIFIER;
    if (!specifier) {
      return config;
    }

    const team = process.env.RUMI_IOS_TEAM_ID || config.ios?.appleTeamId;
    if (!team) {
      throw new Error(
        "withIosManualSigning needs a team id — set RUMI_IOS_TEAM_ID or ios.appleTeamId."
      );
    }

    const project = config.modResults;
    const target = project.getFirstTarget();
    if (!target?.firstTarget?.buildConfigurationList) {
      throw new Error(
        "withIosManualSigning could not find the app target — the prebuild template " +
          "changed shape. Check ios/*.xcodeproj after prebuild."
      );
    }

    const lists = project.pbxXCConfigurationList();
    const section = project.pbxXCBuildConfigurationSection();
    const uuids: string[] = (
      lists[target.firstTarget.buildConfigurationList]?.buildConfigurations ?? []
    ).map((entry: { value: string }) => entry.value);

    // Values are written into the pbxproj verbatim, so anything containing a space (the
    // profile name, "Apple Distribution") has to carry its own quotes.
    const quote = (value: string) => `"${value}"`;
    let patched = 0;

    for (const uuid of uuids) {
      const buildConfig = section[uuid];
      // Release only: Debug stays on automatic signing so `expo run:ios` still works.
      if (!buildConfig?.buildSettings || buildConfig.name !== "Release") {
        continue;
      }

      const settings = buildConfig.buildSettings;
      settings.CODE_SIGN_STYLE = quote("Manual");
      settings.DEVELOPMENT_TEAM = quote(team);
      settings.PROVISIONING_PROFILE_SPECIFIER = quote(specifier);
      settings.CODE_SIGN_IDENTITY = quote("Apple Distribution");
      // Xcode prefers the sdk-scoped key when both are present, and the template sets it to
      // a development identity — leaving it behind would silently win over the line above.
      settings['"CODE_SIGN_IDENTITY[sdk=iphoneos*]"'] = quote("Apple Distribution");
      patched += 1;
    }

    if (patched === 0) {
      throw new Error(
        "withIosManualSigning found no Release configuration on the app target — the " +
          "prebuild template changed shape. Check ios/*.xcodeproj after prebuild."
      );
    }

    return config;
  });

export default withIosManualSigning;
