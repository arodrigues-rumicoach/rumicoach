// Same reasoning as the mixpanel-react-native mock: tests must not start a
// recorder or reach for the native module behind it. Kept alongside it so the
// two stay in step.
const MPSessionReplayMask = { Text: 'text', Web: 'web', Map: 'map', Image: 'image' }

const MPDataResidency = {
  US: 'https://api.mixpanel.com',
  EU: 'https://api-eu.mixpanel.com',
  IN: 'https://api-in.mixpanel.com',
}

class MPSessionReplayConfig {
  constructor(options = {}) {
    Object.assign(this, options)
  }
}

const MPSessionReplay = {
  initialize: jest.fn(() => Promise.resolve()),
  startRecording: jest.fn(() => Promise.resolve()),
  stopRecording: jest.fn(() => Promise.resolve()),
  identify: jest.fn(() => Promise.resolve()),
  isRecording: jest.fn(() => Promise.resolve(false)),
  getReplayId: jest.fn(() => Promise.resolve(null)),
  flush: jest.fn(() => Promise.resolve()),
}

module.exports = {
  MPSessionReplay,
  MPSessionReplayConfig,
  MPSessionReplayMask,
  MPDataResidency,
}
