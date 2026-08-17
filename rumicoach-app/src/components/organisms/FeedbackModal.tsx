import { memo, useCallback, useEffect, useRef, useState } from 'react'
import { Modal, View, Image, StyleSheet, Platform, KeyboardAvoidingView, TouchableOpacity, TouchableWithoutFeedback, Keyboard, useWindowDimensions } from 'react-native'
import { Text, TextArea, XStack } from 'tamagui'
import { BlurView } from 'expo-blur'
import { useSegments } from 'expo-router'
import { Bug, MessageSquare, Lightbulb, ImagePlus, X } from 'lucide-react-native'
import { api } from '@/api/client'
import { collectFeedbackContext, type FeedbackContext } from '@/utils/feedbackContext'
import { pickFeedbackImages, MAX_IMAGES, type PickedImage } from '@/utils/feedbackImages'
import { ThemedButton } from '@/components/atoms'
import { useSettings } from '@/hooks/useSettings'
import i18n from '@/i18n'
import Reanimated, { FadeIn, FadeOut, SlideInDown } from 'react-native-reanimated'
import { trackFeedbackSubmitted } from '@/analytics'

type FeedbackCategory = 'bug' | 'feedback' | 'feature'

interface FeedbackModalProps {
  visible: boolean
  onClose: () => void
  onSubmitSuccess?: () => void
  /** Set when the shake gesture opened this, not a menu — reported on submit. */
  openedByShake?: boolean
}

const categories: { key: FeedbackCategory; icon: typeof Bug; labelKey: string; fallback: string }[] = [
  { key: 'bug', icon: Bug, labelKey: 'feedback_bug_report', fallback: 'Bug Report' },
  { key: 'feedback', icon: MessageSquare, labelKey: 'feedback_general', fallback: 'Feedback' },
  { key: 'feature', icon: Lightbulb, labelKey: 'feedback_feature_request', fallback: 'Feature Request' },
]

export const FeedbackModal = memo(function FeedbackModal({
  visible,
  onClose,
  onSubmitSuccess,
  openedByShake,
}: FeedbackModalProps) {
  const { colorScheme } = useSettings()
  const segments = useSegments()
  const { width, height } = useWindowDimensions()
  const isWeb = Platform.OS === 'web'
  const cardWidth = isWeb ? Math.min(width * 0.6, 480) : '100%'
  const [category, setCategory] = useState<FeedbackCategory>('feedback')
  const [description, setDescription] = useState('')
  const [images, setImages] = useState<PickedImage[]>([])
  const [picking, setPicking] = useState(false)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const contextRef = useRef<FeedbackContext | null>(null)

  // Captured when the form opens, not when it submits: `screen` is meant to say
  // where the user hit the problem, and by the time they finish typing they may
  // have navigated away.
  useEffect(() => {
    if (visible && !contextRef.current) {
      contextRef.current = collectFeedbackContext(segments.join('/'), { width, height })
    }
  }, [visible, segments, width, height])

  const handleClose = useCallback(() => {
    setCategory('feedback')
    setDescription('')
    setImages([])
    setError(null)
    contextRef.current = null
    onClose()
  }, [onClose])

  const handleAddImages = useCallback(async () => {
    if (picking || images.length >= MAX_IMAGES) return
    setPicking(true)
    setError(null)
    try {
      const result = await pickFeedbackImages(images.length)
      if (result.ok) {
        if (result.images.length) setImages((prev) => [...prev, ...result.images])
        return
      }
      // Say which image is the problem — the server would reject the whole
      // report, so the user needs to know what to swap out.
      if (result.reason === 'permission') {
        setError(i18n.t('feedback_image_permission') || 'Rumi needs access to your photos to attach one.')
      } else if (result.reason === 'too_many') {
        setError(i18n.t('feedback_image_too_many', { count: MAX_IMAGES }) || `You can attach up to ${MAX_IMAGES} images.`)
      } else {
        setError(i18n.t('feedback_image_too_large', { index: result.index ?? 1 }) || `Image ${result.index} is too large, even after compressing.`)
      }
    } catch {
      setError(i18n.t('feedback_image_failed') || "Couldn't attach that image.")
    } finally {
      setPicking(false)
    }
  }, [picking, images.length])

  const handleRemoveImage = useCallback((index: number) => {
    setImages((prev) => prev.filter((_, i) => i !== index))
  }, [])

  const handleSubmit = useCallback(async () => {
    if (loading || !description.trim()) return
    setLoading(true)
    setError(null)
    try {
      await api.post('/feedback', {
        category,
        description: description.trim(),
        ...(images.length ? { images: images.map((i) => i.base64) } : {}),
        ...(contextRef.current ? { context: contextRef.current } : {}),
      })
      // Category and shape only. The description is the report itself and stays
      // in the feedback API, which is where someone will actually read it.
      trackFeedbackSubmitted(category, images.length > 0, !!openedByShake)
      onSubmitSuccess?.()
      handleClose()
    } catch (err) {
      // The server's 400 messages name the offending field and are written to be
      // read, so show them verbatim rather than a generic failure.
      const apiMessage = (err as { response?: { data?: { message?: string; error?: string } } })
        .response?.data?.message
        ?? (err as { response?: { data?: { error?: string } } }).response?.data?.error
      setError(apiMessage || (i18n.t('feedback_submit_error') || 'Failed to submit feedback'))
    } finally {
      setLoading(false)
    }
  }, [loading, category, description, images, handleClose, onSubmitSuccess, openedByShake])

  const cardContent = (
    <>
      <View style={styles.header}>
        <Text fontSize={20} fontWeight="700" color="#262220" textAlign="center">
          {i18n.t('send_feedback') || 'Help us improve'}
        </Text>
        <Text fontSize={14} color="#524B46" textAlign="center" lineHeight={20}>
          {i18n.t('feedback_settings_subtitle') || 'Report a bug, share feedback, or request a feature.'}
        </Text>
      </View>

      <View style={styles.categoryRow}>
        {categories.map(({ key, icon: Icon, labelKey, fallback }) => {
          const active = category === key
          return (
            <TouchableWithoutFeedback key={key} onPress={() => setCategory(key)}>
              <View
                style={[
                  styles.chip,
                  {
                    backgroundColor: active ? colorScheme.primary : 'rgba(255,255,255,0.75)',
                    borderColor: active ? colorScheme.primary : 'rgba(0,0,0,0.15)',
                  },
                ]}
              >
                <Icon size={14} color={active ? '#fff' : '#524B46'} />
                <Text fontSize={12} fontWeight="600" color={active ? '#fff' : '#524B46'}>
                  {i18n.t(labelKey) || fallback}
                </Text>
              </View>
            </TouchableWithoutFeedback>
          )
        })}
      </View>

      <TextArea
        value={description}
        onChangeText={setDescription}
        placeholder={i18n.t('feedback_description_placeholder') || 'Describe your issue or idea...'}
        placeholderTextColor={"#9CA3AF" as any}
        backgroundColor="#F9FAFB"
        borderWidth={1.5}
        borderColor="#E5E7EB"
        borderRadius={12}
        color="#262220"
        fontSize={15}
        minHeight={120}
        maxLength={2000}
        multiline
        numberOfLines={5}
        textAlignVertical="top"
        padding={12}
      />

      {/* Thumbnails first, then the add button — so the button keeps moving
          right as they fill up and disappears once the third is attached. */}
      <XStack gap={8} alignItems="center" flexWrap="wrap">
        {images.map((img, i) => (
          <View key={img.uri} style={styles.thumbWrap}>
            <Image source={{ uri: img.uri }} style={styles.thumb} />
            <TouchableOpacity
              style={styles.thumbRemove}
              onPress={() => handleRemoveImage(i)}
              hitSlop={8}
              accessibilityLabel={i18n.t('feedback_image_remove') || 'Remove image'}
            >
              <X size={12} color="#fff" />
            </TouchableOpacity>
          </View>
        ))}

        {images.length < MAX_IMAGES && (
          <TouchableOpacity
            style={styles.addImage}
            onPress={handleAddImages}
            disabled={picking}
            activeOpacity={0.7}
          >
            <ImagePlus size={18} color="#524B46" />
            <Text fontSize={12} fontWeight="600" color="#524B46">
              {images.length === 0
                ? (i18n.t('feedback_add_image') || 'Add image')
                : `${images.length}/${MAX_IMAGES}`}
            </Text>
          </TouchableOpacity>
        )}
      </XStack>

      {error && (
        <View style={styles.errorBox}>
          <Text color="#ef4444" fontSize={13} textAlign="center">{error}</Text>
        </View>
      )}

      <View style={styles.buttonContainer}>
        <ThemedButton
          variant="solid"
          fullWidth
          onPress={handleSubmit}
          disabled={loading || !description.trim()}
          loading={loading}
        >
          {loading
            ? (i18n.t('feedback_submitting') || 'Submitting...')
            : (i18n.t('feedback_submit') || 'Submit')}
        </ThemedButton>
        <ThemedButton
          variant="glass"
          fullWidth
          onPress={handleClose}
          disabled={loading}
        >
          {i18n.t('cancel') || 'Cancel'}
        </ThemedButton>
      </View>
    </>
  )

  return (
    <Modal visible={visible} transparent animationType="none" onRequestClose={handleClose}>
      <KeyboardAvoidingView
        style={styles.overlay}
        behavior={Platform.OS === 'ios' ? 'padding' : 'height'}
        keyboardVerticalOffset={Platform.OS === 'ios' ? 40 : 0}
      >
        <TouchableWithoutFeedback onPress={Keyboard.dismiss}>
          <Reanimated.View
            entering={FadeIn.duration(200)}
            exiting={FadeOut.duration(150)}
            style={styles.overlayContent}
          >
            <Reanimated.View
              entering={SlideInDown.duration(300).springify().damping(20).stiffness(200)}
              exiting={FadeOut.duration(150)}
              style={[styles.card, { width: cardWidth }]}
            >
              {isWeb ? (
                <Reanimated.View
                  entering={FadeIn.duration(300).delay(150)}
                  exiting={FadeOut.duration(150)}
                  style={styles.cardContent}
                >
                  {cardContent}
                </Reanimated.View>
              ) : (
                <BlurView style={styles.blurCard} tint="light" intensity={40}>
                  <Reanimated.View
                    entering={FadeIn.duration(300).delay(150)}
                    exiting={FadeOut.duration(150)}
                    style={{ backgroundColor: 'transparent', gap: 16 }}
                  >
                    {cardContent}
                  </Reanimated.View>
                </BlurView>
              )}
            </Reanimated.View>
          </Reanimated.View>
        </TouchableWithoutFeedback>
      </KeyboardAvoidingView>
    </Modal>
  )
})

const styles = StyleSheet.create({
  overlay: {
    flex: 1,
    backgroundColor: 'rgba(0,0,0,0.4)',
  },
  overlayContent: {
    flex: 1,
    alignItems: 'center',
    justifyContent: 'center',
    padding: 20,
  },
  card: {
    width: '100%',
    maxWidth: 420,
    borderRadius: 20,
    overflow: 'hidden',
    boxShadow: '0px 8px 24px rgba(0,0,0,0.15)',
  },
  cardContent: {
    width: '100%',
    borderRadius: 20,
    borderWidth: 1,
    borderColor: 'rgba(255,255,255,0.4)',
    padding: 24,
    gap: 16,
    backgroundColor: 'rgba(255, 255, 255, 0.95)',
    ...(Platform.OS === 'web' ? { backdropFilter: 'blur(16px)' } : {}),
  },
  blurCard: {
    width: '100%',
    borderRadius: 20,
    overflow: 'hidden',
    padding: 24,
    backgroundColor: 'rgba(255, 255, 255, 0.92)',
  },
  header: {
    gap: 6,
  },
  categoryRow: {
    flexDirection: 'row',
    gap: 8,
  },
  chip: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 5,
    borderRadius: 100,
    borderWidth: 1,
    paddingHorizontal: 12,
    paddingVertical: 7,
  },
  errorBox: {
    backgroundColor: 'rgba(239,68,68,0.1)',
    padding: 12,
    borderRadius: 8,
    borderWidth: 1,
    borderColor: 'rgba(239,68,68,0.3)',
  },
  addImage: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 6,
    height: 56,
    paddingHorizontal: 12,
    borderRadius: 10,
    borderWidth: 1.5,
    borderStyle: 'dashed',
    borderColor: '#E5E7EB',
    backgroundColor: '#F9FAFB',
  },
  thumbWrap: {
    width: 56,
    height: 56,
  },
  thumb: {
    width: 56,
    height: 56,
    borderRadius: 10,
    borderWidth: 1,
    borderColor: '#E5E7EB',
  },
  thumbRemove: {
    position: 'absolute',
    top: -6,
    right: -6,
    width: 20,
    height: 20,
    borderRadius: 10,
    backgroundColor: '#262220',
    alignItems: 'center',
    justifyContent: 'center',
  },
  buttonContainer: {
    flexDirection: 'column',
    gap: 8,
    marginTop: 4,
  },
})
