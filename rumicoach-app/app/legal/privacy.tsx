import { WebView } from 'react-native-webview'
import { YStack, Button, Text } from 'tamagui'
import { router } from 'expo-router'
import { ArrowLeft } from 'lucide-react-native'
import { isWeb } from '../../src/adapters/platform'
import i18n from '../../src/i18n'

// Same as the terms screen: the website owns the copy, the app frames it. The old
// rumiapp.com/privacy-policy 404s.
//
// /privacy-policy, not /privacy — the website serves both, but /privacy is the marketing
// page ("Privacy Built-In") and /privacy-policy is the legal document. This is also the URL
// filed as the app's Privacy Policy in App Store Connect, so the two cannot disagree.
//
// The trailing /index.html is the canonical path: /privacy-policy 301s here, and a redirect
// is one more thing to fail inside a WebView during app review.
const PRIVACY_URL = 'https://rumi.coach/privacy-policy/index.html'

export default function PrivacyScreen() {
  return (
    <YStack flex={1} backgroundColor="$background">
      <YStack padding="$3" backgroundColor="$backgroundSecondary">
        <Button
          variant="outlined"
          alignSelf="flex-start"
          icon={<ArrowLeft size={20} />}
          onPress={() => router.back()}
        >
          {i18n.t('back') || 'Back'}
        </Button>
      </YStack>
      {isWeb ? (
        <iframe src={PRIVACY_URL} style={{ flex: 1, border: 'none', width: '100%' }} />
      ) : (
        <WebView source={{ uri: PRIVACY_URL }} style={{ flex: 1 }} />
      )}
    </YStack>
  )
}
