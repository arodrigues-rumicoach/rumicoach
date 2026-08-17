import { getSessionDetails } from '../constants'

// Every value the backend can put in `session` / `sessions[].session` must resolve to a
// card. The growth carousel drops anything getSessionDetails does not know (`if (details)`),
// so an unmapped type is not a visible error — the card just silently disappears. That is
// exactly what happened to the check-in: the config key was still the long-retired
// 'checkin_daily' while the API sends 'checkin'.
const API_SESSION_TYPES = [
  'onboarding',
  'session_vision',
  'session_movement',
  'session_values',
  'session_energy',
  'session_decisions',
  'session_beliefs',
  'session_identity',
  'session_acceptance',
  'checkin',
]

describe('getSessionDetails', () => {
  it.each(API_SESSION_TYPES)('resolves a full card for %s', (type: string) => {
    const details = getSessionDetails(type)
    expect(details).toBeDefined()
    expect(details?.title).toBeTruthy()
    expect(details?.subtitle).toBeTruthy()
    expect(details?.actionText).toBeTruthy()
    expect(details?.imageUrl).toBeTruthy()
  })

  it('returns undefined for a type it does not know', () => {
    expect(getSessionDetails('session_not_a_real_thing')).toBeUndefined()
  })

  // The check-in card's copy is translated in every locale under the 'checkin_daily' key
  // family. Deriving the key from the type name would look up 'session_checkin_subtitle',
  // which does not exist, and silently serve the English fallback to 19 locales.
  it('reads the check-in copy from the key family that is actually translated', () => {
    const details = getSessionDetails('checkin')
    expect(details?.subtitle).toBe('A moment just for you')
    expect(details?.actionText).toBe('Begin')
    // The real regression this guards: a wrong key makes i18n-js return a
    // truthy '[missing "..." translation]', which sails past the `||` fallback
    // and ships to the UI. Assert on the shape, not just today's wording.
    expect(details?.subtitle).not.toMatch(/missing/)
    expect(details?.actionText).not.toMatch(/missing/)
  })
})
