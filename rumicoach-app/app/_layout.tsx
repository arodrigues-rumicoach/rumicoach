import '../src/styles/global.css'

import { patchConsoleError } from '../src/utils/patchConsole'

import { Component, useContext, useEffect, useState } from 'react'
import type { ErrorInfo, ReactNode } from 'react'
import { Stack, useSegments, useRouter, DarkTheme, ThemeProvider } from 'expo-router'
import { StatusBar } from 'expo-status-bar'
import { TamaguiProvider } from '@tamagui/core'
import { SafeAreaProvider } from 'react-native-safe-area-context'
import { useFonts } from 'expo-font'
import * as SplashScreen from 'expo-splash-screen'
import appConfig from '../tamagui.config'
import { I18nProvider , useI18n } from '../src/i18n'
import { AuthProvider } from '../src/context/AuthContext'
import { needsConsent } from '../src/consent/consents'
import { RevenueCatProvider } from '../src/context/RevenueCatContext'
import { useRevenueCat } from '../src/hooks/useRevenueCat'
import { paywallModeFor, paywallRoute } from '../src/subscriptions/gate'
import { isWeb } from '../src/adapters/platform'
import { SettingsProvider } from '../src/context/SettingsContext'
import { AudioProvider } from '../src/context/AudioContext'
import { SessionProvider, SessionContext } from '../src/context/SessionContext'
import { AlertProvider } from '../src/context/AlertContext'
import { FeedbackProvider } from '../src/context/FeedbackProvider'
import { useAuth } from '../src/hooks/useAuth'
import { Config } from '../src/config'
import { authBackendUrl, regionBackendUrl } from '../src/api/backend-url'
import { trackScreenView, registerBackgroundMessageHandler, registerForegroundMessageHandler } from '../src/firebase'
import { PostHogProvider } from 'posthog-react-native'
// The client itself, only so the provider can be handed it; the events API is
// separate because it fans out to Mixpanel too.
import { posthog } from '../src/analytics/posthog'
import { captureScreen } from '../src/analytics'
import { Toast } from '@/components/molecules'
import { VideoBackground, AmbientAudioPlayer } from '@/components/organisms'
import { ensureThemeAssets } from '../src/utils/assetManager'
import { useSettings } from '../src/hooks/useSettings'
patchConsoleError()

// Keep the splash screen visible while custom fonts load on native.
SplashScreen.preventAutoHideAsync()

const navigationTheme = {
  ...DarkTheme,
  colors: {
    ...DarkTheme.colors,
    background: 'transparent',
    card: 'transparent',
  },
}

console.log('[CONFIG]', JSON.stringify({
  authBackendUrl,
  defaultRegionBackendUrl: regionBackendUrl('eu'),
  GOOGLE_CLIENT_ID_IOS: Config.GOOGLE_CLIENT_ID_IOS,
  GOOGLE_CLIENT_ID_ANDROID: Config.GOOGLE_CLIENT_ID_ANDROID,
  GOOGLE_CLIENT_ID_WEB: Config.GOOGLE_CLIENT_ID_WEB,
}, null, 2))

function useScreenTracking() {
  const segments = useSegments()

  useEffect(() => {
    if (segments.length > 0) {
      const screen = segments.join('/')
      trackScreenView(screen)
      // PostHog's captureScreens autocapture is off (see the provider below), so
      // the $screen event is sent from here — the same signal Firebase gets.
      captureScreen(screen)
    }
  }, [segments])
}

/**
 * Wraps the app in PostHog only when there is a client to wrap it with.
 *
 * <PostHogProvider> builds its own client from `apiKey` when it isn't handed
 * one, and that constructor throws on web and without a key — inside render,
 * so it takes the whole app down rather than just disabling analytics. Skipping
 * the provider entirely is the only way to opt out.
 *
 * captureScreens is off because that autocapture hooks @react-navigation's
 * NavigationContainer, which expo-router owns and never exposes; useScreenTracking
 * sends $screen instead. captureTouches stays off (the SDK's own default) — it
 * reads element labels to name events, and in this app those are user content.
 */
function Analytics({ children }: { children: ReactNode }) {
  if (!posthog) return <>{children}</>
  return (
    <PostHogProvider client={posthog} autocapture={{ captureScreens: false, captureTouches: false }}>
      {children}
    </PostHogProvider>
  )
}

class RenderBoundary extends Component<{ children: ReactNode }, { error: unknown }> {
  constructor(props: { children: ReactNode }) {
    super(props)
    this.state = { error: null }
  }

  static getDerivedStateFromError(error: unknown) {
    return { error }
  }

  componentDidCatch(error: unknown, info: ErrorInfo) {
    console.error('[RENDER BOUNDARY]', error, info.componentStack)
  }

  render() {
    if (this.state.error) {
      const msg = this.state.error instanceof Error ? this.state.error.message : String(this.state.error)
      console.error('[RENDER BOUNDARY ERROR]', msg)
      return null
    }
    return this.props.children
  }
}

function NavigationHandler() {
  const { pendingNavigation, clearPendingNavigation } = useContext(SessionContext)!
  const { customerInfo } = useRevenueCat()
  const router = useRouter()

  useEffect(() => {
    if (!pendingNavigation) return

    if (pendingNavigation === 'memories') {
      router.navigate('/(tabs)/memories')
    } else if (pendingNavigation === 'wheel_of_life' || pendingNavigation === 'session') {
      router.navigate('/(tabs)/session')
    } else if (pendingNavigation === 'tasks' || pendingNavigation === 'journey') {
      router.navigate('/(tabs)/journey')
    } else if (pendingNavigation === 'profile') {
      router.navigate('/(tabs)/profile')
    } else if (pendingNavigation === 'paywall') {
      // The server refused a session start over the socket. Someone already paying has
      // run out and wants more minutes; everyone else needs the membership first.
      // push(), not navigate(): dismissing the paywall returns to wherever they tried
      // to start the session.
      router.push(paywallRoute(paywallModeFor(customerInfo)) as '/paywall')
    }

    clearPendingNavigation()
  }, [pendingNavigation, clearPendingNavigation, router, customerInfo])

  return null
}

export default function RootLayout() {
  const [notification, setNotification] = useState<string | null>(null)
  const segments: string[] = useSegments()
  const [fontsLoaded, fontError] = useFonts({
    'Manrope-Regular': require('../assets/fonts/Manrope-Regular.ttf'),
    'Manrope-SemiBold': require('../assets/fonts/Manrope-SemiBold.ttf'),
    'Manrope-Bold': require('../assets/fonts/Manrope-Bold.ttf'),
  })

  useEffect(() => {
    if (fontsLoaded || fontError) {
      SplashScreen.hideAsync()
    }
  }, [fontsLoaded, fontError])

  useEffect(() => {
    registerBackgroundMessageHandler()
  }, [])

  useEffect(() => {
    const unsubscribe = registerForegroundMessageHandler((title, body) => {
      setNotification(`${title}: ${body}`)
    })
    return () => unsubscribe()
  }, [])

  // Don't render the app until fonts are ready (or fail) to avoid a font flash.
  if (!fontsLoaded && !fontError) {
    return null
  }

  return (
    <RenderBoundary>
      <Analytics>
      <SafeAreaProvider style={{ flex: 1 }}>
        <I18nProvider>
          <AuthProvider>
            <RevenueCatProvider>
              <SettingsProvider>
              <AudioProvider>
                <SessionProvider>
                  <TamaguiProvider config={appConfig} defaultTheme="dark">
                    <AlertProvider>
                     <FeedbackProvider>
                      <StatusBar style="light" />
                      <ScreenTracker />
                      <ThemePreloader />
                      <HtmlLangUpdater />
                      {notification && (
                        <Toast
                          message={notification}
                          type="success"
                          onClose={() => setNotification(null)}
                          duration={5000}
                        />
                      )}
                      <NavigationGuard />
                      <ThemeProvider value={navigationTheme}>
                        <VideoBackground active={segments[0] === '(tabs)'}>
                          <AmbientAudioPlayer />
                          <Stack screenOptions={{ headerShown: false, animation: 'slide_from_right', contentStyle: { backgroundColor: 'transparent' } }}>
                            <Stack.Screen name="index" />
                            <Stack.Screen name="(auth)" />
                            <Stack.Screen name="(tabs)" />
                            <Stack.Screen name="(settings)" />
                            <Stack.Screen name="legal" />
                            <Stack.Screen name="streak" />
                            {/* The paywall is a decision to make or dismiss, not a place in the
                                navigation hierarchy, so it opens over whatever screen asked for
                                it rather than replacing it.

                                Split by platform because the two need different things. On web,
                                `modal` swaps the screen out entirely, leaving the paywall over an
                                empty background — it reads as a page you navigated to, which is
                                exactly what it should not be; transparentModal keeps the screen
                                underneath mounted and visible behind the scrim. On native,
                                `modal` is the iOS sheet, and that is worth keeping: it brings
                                swipe-to-dismiss, which is the only way out of a paywall whose
                                design carries no close button. */}
                            <Stack.Screen
                              name="paywall"
                              options={{
                                presentation: isWeb ? 'transparentModal' : 'modal',
                                animation: isWeb ? 'fade' : 'slide_from_bottom',
                              }}
                            />
                          </Stack>
                          <NavigationHandler />
                        </VideoBackground>
                      </ThemeProvider>
                     </FeedbackProvider>
                    </AlertProvider>
                  </TamaguiProvider>
                </SessionProvider>
              </AudioProvider>
            </SettingsProvider>
            </RevenueCatProvider>
          </AuthProvider>
        </I18nProvider>
      </SafeAreaProvider>
      </Analytics>
    </RenderBoundary>
  )
}

function ScreenTracker() {
  useScreenTracking()
  return null
}

function ThemePreloader() {
  const { theme } = useSettings()
  useEffect(() => {
    ensureThemeAssets(theme)
  }, [theme])
  return null
}

function HtmlLangUpdater() {
  const { locale } = useI18n()
  useEffect(() => {
    if (typeof document !== 'undefined') {
      document.documentElement.lang = locale
    }
  }, [locale])
  return null
}

function NavigationGuard() {
  const { user, isLoading, token } = useAuth()
  const segments = useSegments()
  const router = useRouter()
  // useSegments() hands back a fresh array every render, so depending on it
  // directly would re-run the effect constantly. The path is what matters.
  const segmentPath = segments.join('/')

  useEffect(() => {
    if (isLoading) return
    if (!token) return
    if (!user) return

    const inAuthGroup = segments[0] === '(auth)'
    const inSignup = segments.length >= 2 && segments[0] === '(auth)' && segments[1] === 'signup'

    // Anyone signed in has no business sitting in the auth group. Signup is the one exception,
    // because it manages its own step order and would be yanked out mid-flow.
    //
    // This used to also require a complete profile — dateOfBirth, gender, coachGender and
    // coachVoice all set — and that stranded people. An account missing any of the four could
    // sign in successfully, tokens and all, and then simply stay on the verification screen:
    // no redirect, no error, nothing to explain it. test@rumi.coach, the App Review account,
    // has no coachVoice and did exactly that. Reloading appeared to fix it only because
    // app/index.tsx routes on its own without that check.
    //
    // Dropping the condition is safe: coachVoice is optional on the User type, the settings
    // screen already renders without it, and signup derives a default from coachGender when it
    // is missing. An incomplete profile is something to finish in settings, not a reason to
    // trap someone on a sign-in screen.
    if (inAuthGroup && !inSignup) {
      router.replace('/(tabs)/journey')
    }

    // Nobody uses the app without the two required consents on record — the Terms and the
    // one covering voice audio reaching Google's AI. Signup collects them, but signup is not
    // the only door: "Continue with Apple" on the sign-in screen creates an account and
    // lands here directly, having asked for nothing, and that is exactly the route App
    // Review was given before it rejected 1.0 under 5.1.1(i)/5.1.2(i). Gating on the user
    // record rather than on having completed signup also catches every account created
    // before a given consent existed.
    //
    // Signup is excluded because it asks for the same consents in its own step and would be
    // yanked out mid-flow, and the gate obviously cannot redirect to itself.
    if (!inSignup && segments[0] !== 'consent') {
      void needsConsent(user).then(needed => {
        if (needed) router.replace('/consent')
      })
    }
    // isLoading and the segments have to be here, not just user/token. This
    // effect bails out early on either of them, and after Google SSO on Android
    // they routinely settle a tick AFTER the user and token land — the sign-in
    // returns from a separate activity, so auth state and router state arrive in
    // whatever order the OS hands control back. With only [user, token], the one
    // run that mattered hit an early return and nothing ever re-triggered it, so
    // the user sat on the sign-in screen fully authenticated. Relaunching worked
    // because app/index.tsx redirects on its own.
  }, [user, token, isLoading, segmentPath, router])

  return null
}
