import { useState, useRef, useCallback, useEffect, memo } from 'react'
import { Modal, View, StyleSheet, PanResponder, Platform, KeyboardAvoidingView, TouchableWithoutFeedback, Keyboard } from 'react-native'
import { YStack, XStack, Text, TextArea } from 'tamagui'
import { BlurView } from 'expo-blur'
import { Star } from 'lucide-react-native'
import * as Haptics from 'expo-haptics'
import { api } from '@/api/client'
import { ThemedButton } from '@/components/atoms'
import i18n from '@/i18n'
import Reanimated, { FadeIn, FadeOut, SlideInDown } from 'react-native-reanimated'
import { trackSessionRated } from '@/analytics'

interface SessionFeedbackModalProps {
  visible: boolean
  onClose: () => void
  sessionId: string
}

const STAR_SIZE = 38
const STAR_GAP = 10
const TOTAL_STARS = 5

/** Amber for a committed rating; a lighter amber while the pointer is
 *  previewing a change over one, so the two states read differently. */
const STAR_COMMITTED = '#FBBF24'
const STAR_PREVIEW = '#FDE047'

export const SessionFeedbackModal = memo(function SessionFeedbackModal({ visible, onClose, sessionId }: SessionFeedbackModalProps) {
  const [rating, setRating] = useState<number>(0)
  /** Web-only: what the pointer is currently hovering, before any click. */
  const [hoverRating, setHoverRating] = useState<number | null>(null)
  const [feedback, setFeedback] = useState('')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const rowRef = useRef<View>(null)
  const boundsRef = useRef<{ x: number; width: number }>({ x: 0, width: 0 })
  const lastRatingRef = useRef<number>(0)
  /** While dragging, the PanResponder owns the value — don't fight it. */
  const pressingRef = useRef(false)

  const updateBounds = useCallback(() => {
    if (rowRef.current && 'measureInWindow' in rowRef.current) {
      rowRef.current.measureInWindow((x, _y, width) => {
        if (width > 0) {
          boundsRef.current = { x, width }
        }
      })
    }
  }, [])

  const calculateRatingFromPageX = useCallback((pageX: number): number => {
    const { x, width } = boundsRef.current
    if (width <= 0) return 0
    const relativeX = pageX - x
    const frac = Math.max(0, Math.min(1, relativeX / width))
    const raw = frac * TOTAL_STARS
    const rounded = Math.max(0.5, Math.min(TOTAL_STARS, Math.round(raw * 2) / 2))
    return rounded
  }, [])

  const triggerHaptic = useCallback((newRating: number) => {
    if (newRating !== lastRatingRef.current) {
      lastRatingRef.current = newRating
      if (Platform.OS !== 'web') {
        try {
          Haptics.selectionAsync()
        } catch {}
      }
    }
  }, [])

  const updateRatingFromGesture = useCallback((pageX: number) => {
    const newRating = calculateRatingFromPageX(pageX)
    setRating(newRating)
    triggerHaptic(newRating)
    setError(null)
  }, [calculateRatingFromPageX, triggerHaptic])

  const panResponder = useRef(
    PanResponder.create({
      onStartShouldSetPanResponder: () => true,
      onMoveShouldSetPanResponder: () => true,
      onPanResponderGrant: (evt) => {
        pressingRef.current = true
        updateBounds()
        updateRatingFromGesture(evt.nativeEvent.pageX)
      },
      onPanResponderMove: (evt) => {
        updateRatingFromGesture(evt.nativeEvent.pageX)
      },
      onPanResponderRelease: (evt) => {
        pressingRef.current = false
        updateRatingFromGesture(evt.nativeEvent.pageX)
      },
      onPanResponderTerminate: () => {
        pressingRef.current = false
      },
    })
  ).current

  const handleRowLayout = useCallback(() => {
    updateBounds()
  }, [updateBounds])

  // Hover preview. Wired straight to the DOM node because the row already owns
  // a PanResponder, which only sees press gestures — a pointer moving with no
  // button down never reaches it. Native has no hover, so this stays web-only.
  useEffect(() => {
    if (Platform.OS !== 'web' || !visible) return
    const node = rowRef.current as unknown as HTMLElement | null
    if (!node?.addEventListener) return

    const onEnter = () => updateBounds()
    const onMove = (e: MouseEvent) => {
      if (pressingRef.current) return
      setHoverRating(calculateRatingFromPageX(e.pageX))
    }
    const onLeave = () => setHoverRating(null)

    node.addEventListener('mouseenter', onEnter)
    node.addEventListener('mousemove', onMove)
    node.addEventListener('mouseleave', onLeave)
    return () => {
      node.removeEventListener('mouseenter', onEnter)
      node.removeEventListener('mousemove', onMove)
      node.removeEventListener('mouseleave', onLeave)
    }
  }, [visible, updateBounds, calculateRatingFromPageX])

  const handleClose = useCallback(() => {
    setRating(0)
    setHoverRating(null)
    setFeedback('')
    setError(null)
    lastRatingRef.current = 0
    pressingRef.current = false
    onClose()
  }, [onClose])

  const handleSubmit = useCallback(async () => {
    if (loading) return
    setLoading(true)
    setError(null)
    try {
      await api.post(`/chat/sessions/${sessionId}/feedback`, {
        evaluation: rating,
        feedback: feedback.trim() || undefined,
      })
      // The rating, never the text — that is the user talking about their session.
      trackSessionRated(rating, !!feedback.trim())
      handleClose()
    } catch (err) {
      setError((err as Error).message || (i18n.t('feedback_error') || 'Failed to submit feedback'))
    } finally {
      setLoading(false)
    }
  }, [loading, rating, feedback, sessionId, handleClose])

  // Hover wins over the committed value, so the stars track the pointer. The
  // lighter shade only kicks in over an existing rating — that's the case where
  // the user needs to tell "what I picked" from "what I'd get if I clicked".
  const displayRating = hoverRating ?? rating
  const isPreviewingChange = hoverRating !== null && rating > 0
  const starColor = isPreviewingChange ? STAR_PREVIEW : STAR_COMMITTED

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
              style={styles.card}
            >
              <BlurView
                style={{
                  width: '100%',
                  borderRadius: 24,
                  overflow: 'hidden',
                  borderWidth: 1,
                  borderColor: 'rgba(255,255,255,0.08)',
                  paddingHorizontal: 24,
                  paddingVertical: 32,
                  gap: 20,
                  backgroundColor: 'rgba(0,0,0,0.85)',
                }}
                tint="dark"
                intensity={60}
              >
                <YStack gap="$2" alignItems="center">
                  <Text fontSize={22} fontWeight="bold" color="$color" textAlign="center">
                    {i18n.t('feedback_modal_title') || 'How was your session?'}
                  </Text>
                  <Text fontSize={14} color="$colorSecondary" textAlign="center" lineHeight={20}>
                    {i18n.t('feedback_modal_subtitle') || 'Your feedback helps Rumi learn and provide better coaching in the future.'}
                  </Text>
                </YStack>

                <YStack gap={10} alignItems="center">
                  <View
                    ref={rowRef}
                    {...panResponder.panHandlers}
                    onLayout={handleRowLayout}
                    style={styles.starRowContainer}
                  >
                    {Array.from({ length: TOTAL_STARS }, (_, i) => {
                      const starValue = i + 1
                      const isFull = displayRating >= starValue
                      const isHalf = displayRating >= starValue - 0.5 && displayRating < starValue

                      return (
                        <View key={i} style={styles.starButton}>
                          {/* Unfilled Base Star */}
                          <Star
                            size={STAR_SIZE}
                            color="rgba(255,255,255,0.2)"
                            fill="none"
                            strokeWidth={1.5}
                          />

                          {/* Filled Star Overlay (Full or Half Clipped) */}
                          {(isFull || isHalf) && (
                            <View
                              style={[
                                styles.starFilledOverlay,
                                { width: isHalf ? STAR_SIZE / 2 : STAR_SIZE },
                              ]}
                            >
                              <Star
                                size={STAR_SIZE}
                                color={starColor}
                                fill={starColor}
                                strokeWidth={1.5}
                              />
                            </View>
                          )}
                        </View>
                      )
                    })}
                  </View>

                  {/* Same slot either way: the hint explains why Save is still
                      disabled, and swapping it for the badge avoids a jump. */}
                  {displayRating === 0 ? (
                    <Text fontSize={13} color="$colorSecondary" textAlign="center">
                      {i18n.t('feedback_rate_hint') || 'Pick a rating to save'}
                    </Text>
                  ) : (
                    <XStack
                      backgroundColor={isPreviewingChange ? 'rgba(253,224,71,0.16)' : 'rgba(251,191,36,0.12)'}
                      paddingHorizontal={12}
                      paddingVertical={4}
                      borderRadius={12}
                      borderWidth={1}
                      borderColor={isPreviewingChange ? 'rgba(253,224,71,0.35)' : 'rgba(251,191,36,0.25)'}
                    >
                      <Text color={starColor} fontSize={14} fontWeight="600">
                        {displayRating.toFixed(1)} / 5.0
                      </Text>
                    </XStack>
                  )}
                </YStack>

                <TextArea
                  value={feedback}
                  onChangeText={setFeedback}
                  placeholder={i18n.t('feedback_improve_placeholder') || 'How can we improve? (Optional)'}
                  placeholderTextColor={"rgba(255,255,255,0.4)" as any}
                  backgroundColor="rgba(255,255,255,0.06)"
                  borderWidth={1}
                  borderColor="rgba(255,255,255,0.12)"
                  borderRadius={12}
                  color="#fff"
                  fontSize={14}
                  minHeight={90}
                  maxLength={1000}
                  multiline
                  numberOfLines={4}
                  textAlignVertical="top"
                  padding={12}
                />

                {error && (
                  <YStack backgroundColor="rgba(239,68,68,0.1)" padding="$3" borderRadius={8} borderWidth={1} borderColor="rgba(239,68,68,0.3)">
                    <Text color="#ef4444" fontSize={13} textAlign="center">{error}</Text>
                  </YStack>
                )}

                {/* Save leads — it's the action we want; skipping is the way out. */}
                <YStack width="100%" gap={8}>
                  <ThemedButton
                    variant="solid"
                    onPress={handleSubmit}
                    disabled={loading || rating === 0}
                    loading={loading}
                  >
                    {loading ? (i18n.t('feedback_submitting') || 'Saving...') : (i18n.t('feedback_submit') || 'Save')}
                  </ThemedButton>
                  <ThemedButton
                    variant="glass"
                    onPress={handleClose}
                    disabled={loading}
                  >
                    {i18n.t('feedback_skip') || 'Skip'}
                  </ThemedButton>
                </YStack>
              </BlurView>
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
    backgroundColor: 'rgba(0,0,0,0.5)',
  },
  overlayContent: {
    flex: 1,
    alignItems: 'center',
    justifyContent: 'center',
    padding: 20,
  },
  card: {
    width: '100%',
    maxWidth: 380,
    borderRadius: 24,
    overflow: 'hidden',
    boxShadow: '0 20 40 rgba(0,0,0,0.5)',
    elevation: 24,
  },
  starRowContainer: {
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'center',
    gap: STAR_GAP,
    paddingVertical: 6,
    paddingHorizontal: 8,
    ...Platform.select({ web: { cursor: 'pointer' } }),
  },
  starButton: {
    width: STAR_SIZE,
    height: STAR_SIZE,
    alignItems: 'center',
    justifyContent: 'center',
    position: 'relative',
  },
  starFilledOverlay: {
    position: 'absolute',
    top: 0,
    left: 0,
    height: STAR_SIZE,
    overflow: 'hidden',
  },
})

