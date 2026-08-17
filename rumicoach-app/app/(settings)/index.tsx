import { View, ScrollView, TouchableOpacity, StyleSheet } from 'react-native'
import * as WebBrowser from 'expo-web-browser'
import { Text } from 'tamagui'
import { useSafeAreaInsets } from 'react-native-safe-area-context'
import { router } from 'expo-router'
import {
  BarChart3, HelpCircle, Info, LogOut, Palette,
  Shield, User, ChevronRight, Link, Database, MessageSquare
} from 'lucide-react-native'
import { useState, useContext } from 'react'
import { AlertContext } from '../../src/context/AlertContext'
import { Toast } from '@/components/molecules'
import i18n from '../../src/i18n'
import { useSettings } from '../../src/hooks/useSettings'
import { useAuth } from '../../src/hooks/useAuth'
import { useBlurTarget } from '@/context/BlurContext'
import { useFeedback } from '@/context/FeedbackProvider'
import { LANGUAGES } from '../../src/utils/languages'
import { websiteUrl } from '../../src/api/backend-url'
import { GlassCard } from '@/components/atoms'
import Reanimated, { FadeInDown } from 'react-native-reanimated'
function SectionLabel({ children }: { children: string }) {
  return (
    <Text fontSize={12} fontWeight="700" letterSpacing={0.5} color="$onGlassSecondary" textTransform="uppercase">
      {children}
    </Text>
  )
}

export default function SettingsIndexScreen() {
  const insets = useSafeAreaInsets()
  const blurTargetRef = useBlurTarget()
  const { colorScheme } = useSettings()
  const { logout, appLanguage, isAdmin } = useAuth()
  const { showConfirm } = useContext(AlertContext)!
  const { openFeedback } = useFeedback()
  const [message, setMessage] = useState<{ text: string; type: 'success' | 'error' } | null>(null)

  const handleLogout = () => {
    showConfirm({
      title: i18n.t('confirm_logout') || 'Confirm Logout',
      message: i18n.t('logout_confirm_text') || 'Are you sure you want to log out of your account?',
      confirmLabel: i18n.t('logout') || 'Log Out',
      destructive: true,
      type: 'error',
      onConfirm: async () => {
        await logout()
        router.replace('/(auth)/signin')
      },
    })
  }

  const preferenceItems = [
    {
      icon: <Palette size={20} color="#262220" />,
      label: i18n.t('app_settings') || 'App Settings',
      onPress: () => router.push('/(settings)/app'),
    },
    {
      icon: <User size={20} color="#262220" />,
      label: i18n.t('profile_settings') || 'Manage Account',
      onPress: () => router.push('/(settings)/manageAccount'),
    },
    {
      icon: <Database size={20} color="#262220" />,
      label: i18n.t('manage_data') || 'Manage Data',
      onPress: () => router.push('/(settings)/manageData'),
    },
    {
      icon: <Link size={20} color="#262220" />,
      label: i18n.t('integrations') || 'Integrations',
      onPress: () => router.push('/(settings)/integrations'),
    },
    {
      // App Review has to be able to find the purchase surface, and this is the only route
      // to it: the usage screen is what opens the membership and top-up paywalls.
      icon: <BarChart3 size={20} color="#262220" />,
      label: i18n.t('usage') || 'Subscription & Usage',
      onPress: () => router.push('/(settings)/usage'),
    },
    {
      icon: <Text style={{ fontSize: 20 }}>{LANGUAGES.find(l => l.code === appLanguage)?.flag || '🌐'}</Text>,
      label: i18n.t('language') || 'Language',
      onPress: () => router.push('/(settings)/language'),
    }
  ]

  const supportItems = [
    {
      icon: <MessageSquare size={20} color="#262220" />,
      label: i18n.t('send_feedback') || 'Help us improve',
      onPress: openFeedback,
    },
    {
      icon: <HelpCircle size={20} color="#262220" />,
      label: i18n.t('help_support') || 'Help & Support',
      onPress: () => WebBrowser.openBrowserAsync(websiteUrl(`${appLanguage}/support`)),
    },
    {
      icon: <Info size={20} color="#262220" />,
      label: i18n.t('about') || 'About',
      onPress: () => WebBrowser.openBrowserAsync(websiteUrl(`${appLanguage}/about`)),
    },
  ]

  return (
    <View style={{ flex: 1 }}>
      {message && (
        <Toast
          message={message.text}
          type={message.type}
          onClose={() => setMessage(null)}
        />
      )}
      <ScrollView style={styles.scrollArea} contentContainerStyle={[styles.scrollContent, { paddingBottom: insets.bottom + 32 }]}>
        <Reanimated.View entering={FadeInDown.duration(400).springify().damping(20).stiffness(200)}>
          <GlassCard variant="light" borderRadius={18} padding={16} gap={0} blurTarget={blurTargetRef}>
            <SectionLabel>{i18n.t('preferences') || 'Preferences'}</SectionLabel>
            {preferenceItems.map((item, index) => (
              <View key={index}>
                {index > 0 && <View style={styles.separator} />}
                <TouchableOpacity
                  style={styles.menuItem}
                  onPress={item.onPress}
                  activeOpacity={0.7}
                >
                  <View style={[styles.menuIcon, { backgroundColor: 'rgba(0,0,0,0.06)' }]}>
                    {item.icon}
                  </View>
                  <Text fontSize={15} fontWeight="500" color="$onGlass" flex={1}>
                    {item.label}
                  </Text>
                  <ChevronRight size={16} color="#4A4540" />
                </TouchableOpacity>
              </View>
            ))}
          </GlassCard>
        </Reanimated.View>

        <Reanimated.View entering={FadeInDown.delay(100).duration(400).springify().damping(20).stiffness(200)}>
          <GlassCard variant="light" borderRadius={18} padding={16} gap={0} blurTarget={blurTargetRef}>
            <SectionLabel>{i18n.t('support') || 'Support'}</SectionLabel>
            {supportItems.map((item, index) => (
              <View key={index}>
                {index > 0 && <View style={styles.separator} />}
                <TouchableOpacity
                  style={styles.menuItem}
                  onPress={item.onPress}
                  activeOpacity={0.7}
                >
                  <View style={[styles.menuIcon, { backgroundColor: 'rgba(0,0,0,0.06)' }]}>
                    {item.icon}
                  </View>
                  <Text fontSize={15} fontWeight="500" color="$onGlass" flex={1}>
                    {item.label}
                  </Text>
                  <ChevronRight size={16} color="#4A4540" />
                </TouchableOpacity>
              </View>
            ))}
          </GlassCard>
        </Reanimated.View>

        <Reanimated.View entering={FadeInDown.delay(200).duration(400).springify().damping(20).stiffness(200)}>
          <GlassCard variant="light" borderRadius={18} padding={16} gap={0} blurTarget={blurTargetRef}>
            {/* No section header: exporting and deleting data moved to Manage
                Account, next to the profile they act on, and a card labelled
                "Account" says less than its own rows. Logging out stays here —
                reversible and wanted often, so it earns the shallower place.
                Admin sits alongside it rather than among the preferences: it's
                staff tooling, not something the user configures. */}
            {isAdmin && (
              <>
                <TouchableOpacity
                  style={styles.menuItem}
                  onPress={() => router.push('/(settings)/admin')}
                  activeOpacity={0.7}
                >
                  <View style={[styles.menuIcon, { backgroundColor: 'rgba(0,0,0,0.06)' }]}>
                    <Shield size={20} color="#262220" />
                  </View>
                  <Text fontSize={15} fontWeight="500" color="$onGlass" flex={1}>
                    {i18n.t('admin_menu') || 'Admin Menu'}
                  </Text>
                  <ChevronRight size={16} color="#4A4540" />
                </TouchableOpacity>

                <View style={styles.separator} />
              </>
            )}

            <TouchableOpacity
              style={[styles.menuItem, { paddingVertical: isAdmin ? undefined : 0 }]}
              onPress={handleLogout}
              activeOpacity={0.7}
            >
              <View style={[styles.menuIcon, { backgroundColor: 'rgba(239,68,68,0.15)' }]}>
                <LogOut size={20} color="#ef4444" />
              </View>
              <Text fontSize={15} fontWeight="500" color="$error" flex={1}>
                {i18n.t('logout') || 'Log Out'}
              </Text>
            </TouchableOpacity>
          </GlassCard>
        </Reanimated.View>
      </ScrollView>
    </View>
  )
}

const styles = StyleSheet.create({
  scrollArea: {
    flex: 1
  },
  scrollContent: {
    padding: 16,
    paddingBottom: 100,
    gap: 16
  },
  separator: {
    height: 1,
    backgroundColor: 'rgba(0,0,0,0.10)',
  },
  menuItem: {
    flexDirection: 'row',
    alignItems: 'center',
    paddingVertical: 12,
    gap: 12,
  },
  menuIcon: {
    width: 36,
    height: 36,
    borderRadius: 18,
    justifyContent: 'center',
    alignItems: 'center',
  },
})
