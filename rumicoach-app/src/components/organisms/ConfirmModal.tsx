import { memo, useCallback, useEffect, useState } from 'react'
import { Modal, View, TextInput, StyleSheet, Platform, useWindowDimensions } from 'react-native'
import { Text } from '@tamagui/core'
import { BlurView } from 'expo-blur'
import Reanimated, {
  FadeIn,
  FadeOut,
  ZoomIn,
  ZoomOut,
  useSharedValue,
  useAnimatedStyle,
  withRepeat,
  withTiming,
} from 'react-native-reanimated'
import { TriangleAlert, Info, OctagonAlert } from 'lucide-react-native'
import { ThemedButton } from '@/components/atoms/ThemedButton'
import { useSettings } from '@/hooks/useSettings'
import i18n from '@/i18n'

interface ConfirmModalProps {
  visible: boolean
  title: string
  message?: string
  confirmLabel: string
  destructive?: boolean
  onConfirm: () => void | Promise<void>
  onCancel: () => void
  type?: 'error' | 'info' | 'superError'
}

export const ConfirmModal = memo(function ConfirmModal({
  visible,
  title,
  message,
  confirmLabel,
  destructive = false,
  onConfirm,
  onCancel,
  type = 'error',
}: ConfirmModalProps) {
  const { width } = useWindowDimensions()
  const { colorScheme } = useSettings()
  const isWeb = Platform.OS === 'web'
  const cardWidth = isWeb ? Math.min(width * 0.6, 480) : '100%'

  const [deleteInput, setDeleteInput] = useState('')
  const requiredWord = i18n.t('confirm_delete_word') || 'DELETE'
  const isSuperError = type === 'superError'
  const isInputValid = isSuperError && deleteInput.toUpperCase() === requiredWord.toUpperCase()

  const pulseScale = useSharedValue(1)

  useEffect(() => {
    if (visible) {
      pulseScale.value = withRepeat(
        withTiming(1.08, { duration: 600 }),
        -1,
        true,
      )
    } else {
      pulseScale.value = 1
    }
  }, [visible, pulseScale])

  useEffect(() => {
    if (!visible) {
      setDeleteInput('')
    }
  }, [visible])

  const pulseStyle = useAnimatedStyle(() => ({
    transform: [{ scale: pulseScale.value }],
  }))

  const handleConfirm = useCallback(() => {
    onConfirm()
  }, [onConfirm])

  const handleCancel = useCallback(() => {
    onCancel()
  }, [onCancel])

  const getIconConfig = () => {
    if (isSuperError) return { Component: OctagonAlert, color: '#DC2626' }
    if (type === 'error') return { Component: TriangleAlert, color: '#F59E0B' }
    return { Component: Info, color: colorScheme.primary }
  }

  const { Component: IconComponent, color: iconColor } = getIconConfig()

  const instructionText = isSuperError
    ? (i18n.t('type_confirm_instruction') || `Type ${requiredWord} to confirm`)
    : null

  const cardContent = (
    <>
      <View style={styles.iconWrapper}>
        <Reanimated.View
          entering={ZoomIn.springify().damping(12).stiffness(150).delay(200)}
          style={pulseStyle}
        >
          <IconComponent size={40} color={iconColor} strokeWidth={1.8} />
        </Reanimated.View>
      </View>

      <View style={styles.contentWrapper}>
        <Text style={styles.title}>
          {title}
        </Text>

        {message && (
          <Text style={styles.message}>
            {message}
          </Text>
        )}

        {isSuperError && (
          <View style={styles.inputSection}>
            <Text style={styles.instruction}>
              {instructionText}
            </Text>
            <TextInput
              style={[
                styles.textInput,
                isInputValid && styles.textInputValid,
              ]}
              value={deleteInput}
              onChangeText={setDeleteInput}
              placeholder={requiredWord}
              placeholderTextColor="#9CA3AF"
              autoCapitalize="characters"
              autoCorrect={false}
              spellCheck={false}
            />
          </View>
        )}
      </View>

      <View style={styles.buttonContainer}>
        <ThemedButton
          variant={destructive ? 'error' : 'solid'}
          fullWidth
          onPress={handleConfirm}
          disabled={isSuperError && !isInputValid}
          style={destructive ? styles.appleDestructiveButton : undefined}
        >
          {confirmLabel}
        </ThemedButton>
        <ThemedButton
          variant="glass"
          fullWidth
          onPress={handleCancel}
        >
          {i18n.t('cancel') || 'Cancel'}
        </ThemedButton>
      </View>
    </>
  )

  return (
    <Modal
      visible={visible}
      transparent
      animationType="none"
      onRequestClose={handleCancel}
    >
      <Reanimated.View
        entering={FadeIn.duration(250)}
        exiting={FadeOut.duration(200)}
        style={styles.overlay}
      >
        <Reanimated.View
          entering={ZoomIn.duration(350).springify().damping(25).stiffness(200)}
          exiting={ZoomOut.duration(200)}
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
            <BlurView
              style={styles.blurCard}
              tint="light"
              intensity={40}
            >
              <Reanimated.View
                entering={FadeIn.duration(300).delay(150)}
                exiting={FadeOut.duration(150)}
                style={{ backgroundColor: 'transparent', gap: 20 }}
              >
                {cardContent}
              </Reanimated.View>
            </BlurView>
          )}
        </Reanimated.View>
      </Reanimated.View>
    </Modal>
  )
})

const styles = StyleSheet.create({
  overlay: {
    flex: 1,
    alignItems: 'center',
    justifyContent: 'center',
    padding: 20,
    backgroundColor: 'rgba(0,0,0,0.4)',
  },
  card: {
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
    gap: 20,
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
  iconWrapper: {
    alignItems: 'center',
    marginBottom: 4,
  },
  contentWrapper: {
    gap: 8,
  },
  title: {
    fontSize: 18,
    fontWeight: '700',
    textAlign: 'center',
    color: '#262220',
    lineHeight: 24,
  },
  message: {
    fontSize: 15,
    textAlign: 'center',
    lineHeight: 22,
    color: '#524B46',
  },
  inputSection: {
    marginTop: 8,
    gap: 8,
  },
  instruction: {
    fontSize: 14,
    fontWeight: '600',
    textAlign: 'center',
    color: '#DC2626',
  },
  textInput: {
    borderWidth: 1.5,
    borderColor: '#D1D5DB',
    borderRadius: 12,
    paddingHorizontal: 16,
    paddingVertical: 12,
    fontSize: 16,
    fontWeight: '600',
    textAlign: 'center',
    color: '#262220',
    backgroundColor: '#F9FAFB',
    letterSpacing: 2,
  },
  textInputValid: {
    borderColor: '#10B981',
    backgroundColor: '#ECFDF5',
  },
  buttonContainer: {
    flexDirection: 'column',
    gap: 8,
    marginTop: 4,
  },
  appleDestructiveButton: {
    borderRadius: 12,
    height: 44,
    backgroundColor: '#FEE2E2',
    color: '#EF4444',
  },
})
