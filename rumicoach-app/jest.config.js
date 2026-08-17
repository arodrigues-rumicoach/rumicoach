module.exports = {
  preset: 'jest-expo',
  setupFilesAfterEnv: ['@testing-library/jest-native/extend-expect'],
  moduleNameMapper: {
    '^@/(.*)$': '<rootDir>/src/$1',
    '^@shopify/react-native-skia$': '<rootDir>/src/__mocks__/@shopify/react-native-skia.js',
    '^@shopify/react-native-skia/lib/module/web$': '<rootDir>/src/__mocks__/@shopify/react-native-skia.js',
    '^mixpanel-react-native$': '<rootDir>/src/__mocks__/mixpanel-react-native.js',
    '^@mixpanel/react-native-session-replay$': '<rootDir>/src/__mocks__/@mixpanel/react-native-session-replay.js',
  },
  transformIgnorePatterns: [
    'node_modules/(?!((jest-)?react-native|@react-native(-community)?)|expo(nent)?|@expo(nent)?/.*|@expo-google-fonts/.*|react-navigation|@react-navigation/.*|@unimodules/.*|unimodules|sentry-expo|native-base|react-native-svg|tamagui|@tamagui/.*|standard-navigation)',
  ],
}
