import { useEffect, useState, useMemo, useRef, memo } from 'react'
import { XStack, YStack, Text } from 'tamagui'
import i18n from '@/i18n'
import type { SessionSummaryData } from '@/context/SessionContext'
import { FadeSlideIn, Heading } from '@/components/atoms'
import { GlassPanel } from '@/components/molecules/GlassPanel'
import Reanimated, { FadeInDown } from 'react-native-reanimated'

interface SessionSummaryPanelProps {
  data: SessionSummaryData
}

const STAGGER_MS = 450

function SectionLabel({ children }: { children: string }) {
  return (
    <Text
      fontSize={12}
      letterSpacing={1.2}
      textTransform="uppercase"
      color="$onGlassSecondary"
    >
      {children}
    </Text>
  )
}

// A label sitting directly on top of the thing it names. Blocks space their
// children by 8, which is enough to separate fields but too much between a
// label and its own value — hence the tighter pair.
function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <YStack width="100%" gap={4}>
      <SectionLabel>{label}</SectionLabel>
      {children}
    </YStack>
  )
}

// Short self-chosen words (values, qualities) read as a set, not a sentence.
// Accent-tinted, so they must never sit inside an `accent` Block — same fill.
function Pills({ items }: { items: string[] }) {
  return (
    <XStack flexWrap="wrap" gap={8}>
      {items.map((item, index) => (
        <YStack
          key={index}
          paddingHorizontal={12}
          paddingVertical={6}
          borderRadius={999}
          backgroundColor="rgba(16,185,129,0.12)"
          borderWidth={1}
          borderColor="$accentDark"
        >
          <Text fontSize={15} fontWeight="600" color="$onGlass">{item}</Text>
        </YStack>
      ))}
    </XStack>
  )
}

function Block({ children, accent = false }: { children: React.ReactNode; accent?: boolean }) {
  return (
    <YStack
      width="100%"
      borderRadius={16}
      paddingHorizontal={16}
      paddingVertical={14}
      gap={8}
      backgroundColor={accent ? 'rgba(16,185,129,0.12)' : 'rgba(0,0,0,0.05)'}
      borderWidth={accent ? 1 : 0}
      borderColor={accent ? '$accentDark' : undefined}
    >
      {children}
    </YStack>
  )
}

/**
 * End-of-session synthesis shown inside the session screen (like the Wheel of
 * Life panel), driven by the `session_summary` WebSocket message. Blocks are
 * revealed one at a time while Rumi speaks the goodbye, ending on the question
 * the next session will explore. Stored text (vision, values, the identity
 * reflection, insight, commitments) arrives already in the user's language; only
 * the section labels and the next-session question (a `question_key`) are
 * localized here.
 *
 * Every deep session emits this card, and each one fills a different subset of
 * the fields — so every block is conditional. `next_session` in particular is
 * absent on the last session of the journey (Acceptance), where the journey
 * cycles dynamically instead of naming a fixed next step: the card simply ends
 * on the previous block.
 */
export const SessionSummaryPanel = memo(function SessionSummaryPanel({ data }: SessionSummaryPanelProps) {
  const [visibleBlocks, setVisibleBlocks] = useState(1)
  // How many blocks were already on screen before this render's `data` — a later
  // session_summary message (e.g. the next-session card added after the goodbye's
  // bridge) replaces `data` wholesale, so `blocks` is a brand new array each time.
  // Without this, the reveal effect below has no way to tell "grew by one" from
  // "first mount" and always restarted the stagger at 1, unmounting every
  // already-shown block and replaying its entrance animation (QA).
  const revealedRef = useRef(0)

  const blocks = useMemo(() => {
    const result: React.ReactNode[] = []

    if (data.vision) {
      result.push(
        <Block key="vision">
          <SectionLabel>{i18n.t('summary_your_vision')}</SectionLabel>
          <Text fontSize={15} lineHeight={22} color="$onGlass">{data.vision}</Text>
        </Block>,
      )
    }

    // Movement's card opens the story: what has been blocking them (Filipa's spec).
    if (data.main_obstacle) {
      result.push(
        <Block key="obstacle">
          <SectionLabel>{i18n.t('summary_main_obstacle')}</SectionLabel>
          <Text fontSize={15} lineHeight={22} color="$onGlass">{data.main_obstacle}</Text>
        </Block>,
      )
    }

    // The Values session's headline result: the words the user themselves chose,
    // so they get pills rather than a paragraph. No other session type sends them.
    if (data.values && data.values.length > 0) {
      result.push(
        <Block key="values">
          <SectionLabel>{i18n.t('summary_your_values')}</SectionLabel>
          <Pills items={data.values} />
        </Block>,
      )
    }

    // The Identity session's reflection card, laid out like the printed one: the
    // identity the user learned, what it gave and cost them, then — across the
    // divider — the one they are choosing. Every field but the two anchors is
    // optional, so the block stays coherent when the session only got partway.
    // Left deliberately un-accented: its pills carry the accent fill, which would
    // disappear against an accent Block.
    const reflection = data.identity_reflection
    if (reflection && (reflection.learned_identity || reflection.who_becoming)) {
      result.push(
        <Block key="identity">
          {reflection.learned_identity ? (
            <Field label={i18n.t('summary_identity_learned')}>
              <Text fontSize={15} lineHeight={22} color="$onGlass">{reflection.learned_identity}</Text>
            </Field>
          ) : null}
          {reflection.what_it_gave ? (
            <Field label={i18n.t('summary_identity_gave')}>
              <Text fontSize={15} lineHeight={22} color="$onGlass">{reflection.what_it_gave}</Text>
            </Field>
          ) : null}
          {reflection.what_it_costs ? (
            <Field label={i18n.t('summary_identity_costs')}>
              <Text fontSize={15} lineHeight={22} color="$onGlass">{reflection.what_it_costs}</Text>
            </Field>
          ) : null}
          {reflection.who_becoming ? (
            <>
              {/* Marks the turn from the learned identity to the chosen one —
                  suppressed when nothing came before it to divide. */}
              {reflection.learned_identity || reflection.what_it_gave || reflection.what_it_costs ? (
                <YStack width="100%" height={1} backgroundColor="rgba(0,0,0,0.10)" />
              ) : null}
              <Field label={i18n.t('summary_identity_becoming')}>
                <Text fontSize={16} fontWeight="600" lineHeight={24} color="$onGlass">
                  {reflection.who_becoming}
                </Text>
              </Field>
            </>
          ) : null}
          {reflection.qualities && reflection.qualities.length > 0 ? (
            <Field label={i18n.t('summary_identity_qualities')}>
              <Pills items={reflection.qualities} />
            </Field>
          ) : null}
          {reflection.evidence ? (
            <Field label={i18n.t('summary_identity_evidence')}>
              <Text fontSize={15} lineHeight={22} color="$onGlass">{reflection.evidence}</Text>
            </Field>
          ) : null}
        </Block>,
      )
    }

    // The Acceptance session's reflection card, laid out like the printed one:
    // the expectation against the reality, then — across the divider — the
    // control split, and finally the two choices. Every field but the two
    // anchors is optional, so the block stays coherent when the session only
    // got partway. Un-accented for the same reason as the identity block.
    const acceptance = data.acceptance_reflection
    if (acceptance && (acceptance.expected || acceptance.reality)) {
      result.push(
        <Block key="acceptance">
          {acceptance.expected ? (
            <Field label={i18n.t('summary_acceptance_expected')}>
              <Text fontSize={15} lineHeight={22} color="$onGlass">{acceptance.expected}</Text>
            </Field>
          ) : null}
          {acceptance.reality ? (
            <Field label={i18n.t('summary_acceptance_reality')}>
              <Text fontSize={15} lineHeight={22} color="$onGlass">{acceptance.reality}</Text>
            </Field>
          ) : null}
          {acceptance.cannot_control || acceptance.can_influence ? (
            <>
              {/* Marks the turn from the expectation gap to the control split —
                  suppressed when nothing came before it to divide. */}
              {acceptance.expected || acceptance.reality ? (
                <YStack width="100%" height={1} backgroundColor="rgba(0,0,0,0.10)" />
              ) : null}
              {acceptance.cannot_control ? (
                <Field label={i18n.t('summary_acceptance_cannot_control')}>
                  <Text fontSize={15} lineHeight={22} color="$onGlass">{acceptance.cannot_control}</Text>
                </Field>
              ) : null}
              {acceptance.can_influence ? (
                <Field label={i18n.t('summary_acceptance_can_influence')}>
                  <Text fontSize={15} lineHeight={22} color="$onGlass">{acceptance.can_influence}</Text>
                </Field>
              ) : null}
            </>
          ) : null}
          {acceptance.choose_to_accept ? (
            <Field label={i18n.t('summary_acceptance_accept')}>
              <Text fontSize={15} lineHeight={22} color="$onGlass">{acceptance.choose_to_accept}</Text>
            </Field>
          ) : null}
          {acceptance.where_i_act ? (
            <Field label={i18n.t('summary_acceptance_act')}>
              <Text fontSize={16} fontWeight="600" lineHeight={24} color="$onGlass">
                {acceptance.where_i_act}
              </Text>
            </Field>
          ) : null}
          {acceptance.next_step ? (
            <Field label={i18n.t('summary_acceptance_next_step')}>
              <Text fontSize={15} lineHeight={22} color="$onGlass">{acceptance.next_step}</Text>
            </Field>
          ) : null}
        </Block>,
      )
    }

    if (data.priority_area?.name) {
      const score = data.priority_area.score
      const maxScore = data.priority_area.max_score || 10
      result.push(
        <Block key="area">
          <SectionLabel>{i18n.t('summary_priority_area')}</SectionLabel>
          <XStack alignItems="center" justifyContent="space-between">
            <Text fontSize={16} fontWeight="600" color="$onGlass">{data.priority_area.name}</Text>
            {typeof score === 'number' && (
              <Text fontSize={13} color="$onGlassAccent">{score} / {maxScore}</Text>
            )}
          </XStack>
          {typeof score === 'number' && (
            <YStack height={5} borderRadius={3} backgroundColor="rgba(0,0,0,0.10)" overflow="hidden">
              <YStack
                height="100%"
                width={`${Math.max(0, Math.min(100, Math.round((score / maxScore) * 100)))}%`}
                backgroundColor="$accent"
              />
            </YStack>
          )}
        </Block>,
      )
    }

    if (data.key_insight) {
      result.push(
        <Block key="insight">
          <SectionLabel>{i18n.t('summary_biggest_insight')}</SectionLabel>
          <Text fontSize={15} fontStyle="italic" lineHeight={22} color="$onGlass">
            {'"'}{data.key_insight}{'"'}
          </Text>
        </Block>,
      )
    }

    if (data.commitments && data.commitments.length > 0) {
      result.push(
        <Block key="commitments">
          <SectionLabel>{i18n.t('summary_commitments')}</SectionLabel>
          <YStack gap={6}>
            {data.commitments.map((c, index) => (
              <XStack key={index} alignItems="baseline" justifyContent="space-between" gap={8}>
                <Text fontSize={15} flexShrink={1} color="$onGlass">{c.title}</Text>
                {c.date ? <Text fontSize={12} color="$onGlassSecondary">{c.date}</Text> : null}
              </XStack>
            ))}
          </YStack>
        </Block>,
      )
    }

    if (data.behavior_plan?.behavior) {
      result.push(
        <Block key="habit">
          <SectionLabel>{i18n.t('summary_new_habit')}</SectionLabel>
          <Text fontSize={15} color="$onGlass">{data.behavior_plan.behavior}</Text>
          {data.behavior_plan.identity ? (
            <Text fontSize={13} fontStyle="italic" color="$onGlassSecondary">
              {'"'}{data.behavior_plan.identity}{'"'}
            </Text>
          ) : null}
        </Block>,
      )
    }

    // defaultValue guards a key this app build doesn't know yet (backend ahead
    // of the client): i18n-js would otherwise return a literal "[missing ...]"
    // string, which is truthy and renders verbatim inside the accent block.
    const nextQuestion = data.next_session?.question_key
      ? i18n.t(data.next_session.question_key, { defaultValue: '' })
      : null
    if (nextQuestion) {
      result.push(
        <Block key="next" accent>
          <SectionLabel>{i18n.t('summary_next_session')}</SectionLabel>
          <Text fontSize={15} fontWeight="600" lineHeight={22} color="$onGlass">
            {nextQuestion}
          </Text>
        </Block>,
      )
    }

    return result
  }, [data])

  useEffect(() => {
    // Blocks shrinking (or this being the first mount, where the ref starts at 0)
    // means there is nothing meaningful already on screen to preserve — restart the
    // stagger from the top. Otherwise this is blocks being APPENDED to an already
    // fully (or partially) revealed panel: keep everything already shown as-is and
    // only stagger-reveal the newly appended ones.
    const startFrom = blocks.length < revealedRef.current ? 0 : revealedRef.current
    revealedRef.current = blocks.length

    const initialVisible = Math.max(1, startFrom)
    setVisibleBlocks(initialVisible)
    if (blocks.length <= initialVisible) return
    const interval = setInterval(() => {
      setVisibleBlocks(prev => {
        if (prev >= blocks.length) {
          clearInterval(interval)
          return prev
        }
        return prev + 1
      })
    }, STAGGER_MS)
    return () => clearInterval(interval)
  }, [blocks.length])

  if (blocks.length === 0) return null

  return (
    <Reanimated.View entering={FadeInDown.duration(500).springify().damping(20).stiffness(150)}>
      <GlassPanel variant="light" margin={12}>
        <Heading color="$onGlass">{i18n.t('summary_title')}</Heading>
        <YStack width="100%" gap={10}>
          {blocks.slice(0, visibleBlocks).map((block, index) => (
            <FadeSlideIn key={index}>{block}</FadeSlideIn>
          ))}
        </YStack>
      </GlassPanel>
    </Reanimated.View>
  )
})
