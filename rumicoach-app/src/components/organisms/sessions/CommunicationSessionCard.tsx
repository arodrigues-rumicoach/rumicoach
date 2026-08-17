import React from 'react'
import { Text, XStack, YStack } from 'tamagui'
import { Clock, Calendar, Star, Lightbulb, MessageSquare } from 'lucide-react-native'
import type { CommunicationSession } from '@/api'
import i18n from '@/i18n'
import { GlassCard } from '@/components/atoms'

interface Props {
  item: CommunicationSession
  userLanguage?: string | null
}

export function CommunicationSessionCard({ item, userLanguage }: Props) {
  const formatDate = (dateString: string) => {
    try {
      const date = new Date(dateString)
      return date.toLocaleDateString(userLanguage || 'en-US', {
        weekday: 'short',
        month: 'short',
        day: 'numeric',
        hour: '2-digit',
        minute: '2-digit',
      })
    } catch {
      return dateString
    }
  }

  const formatSessionType = (type: string) => {
    return type.split('_').map(word => word.charAt(0).toUpperCase() + word.slice(1)).join(' ')
  }

  let durationStr = ''
  if (item.duration) {
    const durationMins = Math.max(1, Math.round(item.duration / 60))
    durationStr = `${durationMins} min`
  }

  return (
    <GlassCard variant="light" borderRadius={20} padding={0} overflow="hidden" marginBottom={8}>
      <YStack padding={16} gap={14}>
        <XStack justifyContent="space-between" alignItems="center">
          <XStack 
            backgroundColor="rgba(255, 255, 255, 0.2)" 
            paddingHorizontal={12} 
            paddingVertical={4} 
            borderRadius={16}
          >
            <Text fontSize={12} fontWeight="800" color="$onGlass" textTransform="uppercase" letterSpacing={0.5}>
              {formatSessionType(item.sessionType)}
            </Text>
          </XStack>
          
          {item.userEvaluation !== undefined && item.userEvaluation !== null && (
            <XStack alignItems="center" gap={4} backgroundColor="rgba(0, 0, 0, 0.1)" paddingHorizontal={10} paddingVertical={4} borderRadius={16}>
              <Star size={14} color="#FBBF24" fill="#FBBF24" />
              <Text fontSize={13} fontWeight="800" color="$onGlass">
                {item.userEvaluation.toFixed(1)}
              </Text>
            </XStack>
          )}
        </XStack>

        <XStack justifyContent="space-between" alignItems="center">
          <XStack alignItems="center" gap={6}>
            <Calendar size={14} color="$onGlassSecondary" />
            <Text fontSize={14} fontWeight="500" color="$onGlassSecondary">
              {formatDate(item.startTime)}
            </Text>
          </XStack>

          {durationStr ? (
            <XStack alignItems="center" gap={6}>
              <Clock size={14} color="$onGlassSecondary" />
              <Text fontSize={14} fontWeight="500" color="$onGlassSecondary">
                {durationStr}
              </Text>
            </XStack>
          ) : null}
        </XStack>
      </YStack>

      {(item.userSessionInsight || item.userFeedback) && (
        <YStack 
          backgroundColor="rgba(0, 0, 0, 0.08)" 
          padding={16} 
          gap={16}
          borderTopWidth={1}
          borderColor="rgba(255, 255, 255, 0.05)"
        >
          {item.userSessionInsight && (
            <YStack gap={6}>
              <XStack alignItems="center" gap={6}>
                <Lightbulb size={14} color="$onGlassSecondary" />
                <Text fontSize={12} fontWeight="700" color="$onGlassSecondary" textTransform="uppercase" letterSpacing={0.5}>
                  {i18n.t('insight') || 'Key Insight'}
                </Text>
              </XStack>
              <Text fontSize={15} color="$onGlass" lineHeight={22}>
                {item.userSessionInsight}
              </Text>
            </YStack>
          )}

          {item.userFeedback && (
            <YStack gap={6}>
              <XStack alignItems="center" gap={6}>
                <MessageSquare size={14} color="$onGlassSecondary" />
                <Text fontSize={12} fontWeight="700" color="$onGlassSecondary" textTransform="uppercase" letterSpacing={0.5}>
                  {i18n.t('feedback') || 'Your Feedback'}
                </Text>
              </XStack>
              <Text fontSize={15} color="$onGlass" lineHeight={22} fontStyle="italic">
                "{item.userFeedback}"
              </Text>
            </YStack>
          )}
        </YStack>
      )}
    </GlassCard>
  )
}
