import { Stack } from 'expo-router'
import i18n from '../../src/i18n'
import { SettingsBackground, PageHeader } from '@/components/organisms'
import { ContentLayout, WebMaxWidth } from '@/components/templates'
import { ScrollNavProvider } from '@/context/ScrollNavContext'

export default function SettingsLayout() {
  return (
    <ScrollNavProvider>
      <WebMaxWidth>
        <Stack
          screenOptions={{
            headerShown: true,
            headerShadowVisible: false,
            contentStyle: { backgroundColor: 'transparent' },
            animation: 'slide_from_right',
            headerBackVisible: false,
            header: ({ options, back }) => (
              <PageHeader
                title={options.title ?? ''}
                canGoBack={!!back}
              />
            ),
          }}
        >
          <Stack.Screen
            name="index"
            options={{ title: i18n.t('settings') || 'Settings' }}
          />
          <Stack.Screen
            name="manageAccount"
            options={{ title: i18n.t('profile_settings') || 'Profile' }}
          />
          <Stack.Screen
            name="manageData"
            options={{ title: i18n.t('manage_data') || 'Manage Data' }}
          />
          <Stack.Screen
            name="app"
            options={{ title: i18n.t('app_settings') || 'App Settings' }}
          />
          <Stack.Screen
            name="language"
            options={{ title: i18n.t('language') || 'Language' }}
          />
          <Stack.Screen
            name="usage"
            options={{ title: i18n.t('usage') || 'Subscription & Usage' }}
          />
          <Stack.Screen
            name="admin"
            options={{ title: i18n.t('admin_menu') || 'Admin Menu' }}
          />
          <Stack.Screen
            name="app/theme"
            options={{ title: i18n.t('choose_theme') || 'Theme' }}
          />
          <Stack.Screen
            name="integrations/index"
            options={{ title: i18n.t('integrations') || 'Integrations' }}
          />
          <Stack.Screen
            name="integrations/[provider]"
            options={{ title: i18n.t('integrations') || 'Integrations' }}
          />
        </Stack>
      </WebMaxWidth>
    </ScrollNavProvider>
  )
}
