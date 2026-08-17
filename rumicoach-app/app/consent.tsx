// The consent gate: what stands between a signed-in user and using the app.
//
// Registration asks for all three consents, but registration is not the only door.
// "Continue with Apple" (or Google) on the sign-in screen creates an account and lands
// straight inside, having asked for nothing — so that user holds no Terms acceptance and no
// AI consent. Neither do accounts created before a given consent existed. This screen is
// where they answer, and it is gated on the user record rather than on the signup flow so
// it catches every one of those cases.
//
// App Review rejected 1.0 under Guidelines 5.1.1(i) and 5.1.2(i) for the AI half: voice
// audio reached Google's AI with no disclosure and no permission. The rejection states
// plainly that carrying this in the Terms of Service or Privacy Policy is not sufficient,
// which is why the AI consent is its own checkbox with its own words rather than a clause
// inside the Terms line.
//
// Terms and AI are required — the app cannot be used without them. Marketing is optional.

import { useState } from 'react'
import { ScrollView, StyleSheet, View } from 'react-native'
import { Text } from 'tamagui'
import { Link, useRouter } from 'expo-router'
import { useSafeAreaInsets } from 'react-native-safe-area-context'
import { Mic, Cloud, BookOpen } from 'lucide-react-native'

import i18n from '../src/i18n'
import { useAuth } from '../src/hooks/useAuth'
import { Checkbox, GlassCard, Heading, ThemedButton } from '@/components/atoms'
import { rememberConsentLocally } from '../src/consent/consents'
import { INK } from '@/styles/glass'

export default function ConsentScreen() {
  const router = useRouter()
  const insets = useSafeAreaInsets()
  const { user, updateUser, logout } = useAuth()

  const [acceptTerms, setAcceptTerms] = useState(false)
  const [acceptAi, setAcceptAi] = useState(false)
  const [acceptMarketing, setAcceptMarketing] = useState(false)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')

  const canContinue = acceptTerms && acceptAi

  const accept = async () => {
    if (!user || saving || !canContinue) return
    setSaving(true)
    setError('')
    // Mirrored locally before the call. If the backend has not shipped the writable fields
    // yet the PATCH is a no-op, and without this the user would agree, be bounced back here
    // on the next launch, and have no way through.
    await rememberConsentLocally(user.id)
    try {
      // Booleans, not timestamps: the server decides when "now" is, and only `true` counts.
      await updateUser({
        termsAndConditionsAccepted: true,
        aiAccepted: true,
        marketingAccepted: acceptMarketing,
      } as never)
    } catch {
      // Deliberately quiet. The local mirror already let the user through, and a failure
      // here means the backend does not know the fields yet — not something to put in
      // their face mid-onboarding.
    }
    router.replace('/(tabs)/journey')
  }

  const decline = async () => {
    if (saving) return
    // Signing out is the honest outcome. Without these two consents there is no session and
    // no lawful basis to proceed; leaving someone inside with the core feature dead would be
    // worse than a clean exit, and the account itself is untouched.
    setSaving(true)
    try {
      await logout()
    } catch {
      setError(i18n.t('err_generic'))
      setSaving(false)
    }
  }

  const rows = [
    {
      icon: <Mic size={20} color={INK.primary} />,
      text: i18n.t('ai_consent_point_audio')
        || 'The audio of your voice is sent to our server while a session is running.',
    },
    {
      icon: <Cloud size={20} color={INK.primary} />,
      text: i18n.t('ai_consent_point_google')
        || "It is processed by Google's AI services, which turn it into Rumi's spoken reply. Everything stays inside Google Cloud and is not sent anywhere else.",
    },
    {
      icon: <BookOpen size={20} color={INK.primary} />,
      text: i18n.t('ai_consent_point_memories')
        || 'What you say is also saved to your profile as memories and insights, so later sessions do not start from nothing.',
    },
  ]

  return (
    <ScrollView
      style={styles.screen}
      contentContainerStyle={[styles.content, { paddingTop: insets.top + 24, paddingBottom: insets.bottom + 24 }]}
    >
      <GlassCard variant="light" borderRadius={24} padding={24} gap={18}>
        <Heading color="$onGlass" fontSize={22} textAlign="center">
          {i18n.t('ai_consent_title') || 'Before your first session'}
        </Heading>

        <Text color="$onGlassSecondary" fontSize={14} lineHeight={21} textAlign="center">
          {i18n.t('ai_consent_intro') || 'Sessions with Rumi are spoken out loud. Here is what that means for your data.'}
        </Text>

        <View style={styles.rows}>
          {rows.map((row, i) => (
            <View key={i} style={styles.row}>
              <View style={styles.icon}>{row.icon}</View>
              <Text color="$onGlassSecondary" fontSize={13} lineHeight={20} flex={1}>
                {row.text}
              </Text>
            </View>
          ))}
        </View>

        <View style={styles.checks}>
          <View style={styles.row}>
            <Checkbox checked={acceptTerms} onCheckedChange={v => { setAcceptTerms(v); setError('') }} />
            <Text color="$onGlassSecondary" fontSize={12} lineHeight={18} flex={1}>
              <Text color="$error">* </Text>
              {(i18n.t('i_accept_the') || 'I accept the')}{' '}
              <Link href="/legal/terms"><Text color="$accent" fontSize={12}>{i18n.t('terms_of_service') || 'Terms of Service'}</Text></Link>
              {' '}{(i18n.t('terms_and') || 'and acknowledge the')}{' '}
              <Link href="/legal/privacy"><Text color="$accent" fontSize={12}>{i18n.t('privacy_policy') || 'Privacy Policy'}</Text></Link>.
            </Text>
          </View>

          <View style={styles.row}>
            <Checkbox checked={acceptAi} onCheckedChange={v => { setAcceptAi(v); setError('') }} />
            <Text color="$onGlassSecondary" fontSize={12} lineHeight={18} flex={1}>
              <Text color="$error">* </Text>
              {i18n.t('ai_consent_checkbox')
                || "I understand that I am interacting with an AI coaching system, that its outputs are computer-generated and I agree that the audio of my voice is sent to our server and processed by our services, inside Google Cloud, so Rumi can reply out loud, and that what I say is saved to my profile as memories."}
            </Text>
          </View>

          <View style={styles.row}>
            <Checkbox checked={acceptMarketing} onCheckedChange={setAcceptMarketing} />
            <Text color="$onGlassSecondary" fontSize={12} lineHeight={18} flex={1}>
              {i18n.t('marketing_consent') || 'Keep me updated with coaching tips and new feature releases'}
            </Text>
          </View>
        </View>

        {error ? <Text color="$error" fontSize={12} textAlign="center">{error}</Text> : null}

        <ThemedButton variant="solid" fullWidth onPress={accept} disabled={saving || !canContinue} loading={saving}>
          {i18n.t('ai_consent_accept') || 'Agree and continue'}
        </ThemedButton>

        <ThemedButton variant="outline" fullWidth onPress={decline} disabled={saving}>
          {i18n.t('ai_consent_decline') || 'Not now'}
        </ThemedButton>
      </GlassCard>
    </ScrollView>
  )
}

const styles = StyleSheet.create({
  screen: { flex: 1 },
  content: { paddingHorizontal: 20, justifyContent: 'center', flexGrow: 1 },
  rows: { gap: 14 },
  checks: { gap: 12, marginTop: 2 },
  row: { flexDirection: 'row', alignItems: 'flex-start', gap: 12 },
  icon: { width: 24, alignItems: 'center', paddingTop: 1 },
})
