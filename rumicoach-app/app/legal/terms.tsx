import { WebView } from 'react-native-webview'
import { YStack, Button } from 'tamagui'
import { router } from 'expo-router'
import { ArrowLeft } from 'lucide-react-native'
import { isWeb } from '../../src/adapters/platform'
import i18n from '../../src/i18n'

// The website is the canonical home of the legal copy; the app only frames it. The path is
// the website's own route (rumicoach-website), not a guess — rumiapp.com/terms-of-service,
// which this used to point at, 404s.
//
// The trailing /index.html is the canonical path: /terms-and-conditions 301s here, and a
// redirect is one more thing to fail inside a WebView during app review.
const TERMS_URL = 'https://rumi.coach/terms-and-conditions/index.html'

export default function TermsScreen() {
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
        <iframe src={TERMS_URL} style={{ flex: 1, border: 'none', width: '100%' }} />
      ) : (
        <WebView source={{ uri: TERMS_URL }} style={{ flex: 1 }} />
      )}
    </YStack>
  )
}
