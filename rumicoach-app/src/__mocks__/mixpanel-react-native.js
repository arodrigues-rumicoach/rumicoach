// mixpanel-react-native ships untranspiled ESM, which Jest will not parse under
// this project's transformIgnorePatterns. Whitelisting it there would work, but
// mocking is the better answer regardless: tests have no business starting an
// analytics SDK, reaching for native modules, or queueing events to a real
// project. Every method is a no-op spy so assertions can still reach them.
const people = { set: jest.fn(), setOnce: jest.fn(), increment: jest.fn() }

class Mixpanel {
  constructor() {
    this.autocapture = { trackScreenView: jest.fn(), trackScreenLeave: jest.fn() }
  }

  init() { return Promise.resolve() }
  track() { }
  identify() { }
  getPeople() { return people }
  registerSuperProperties() { }
  reset() { }
  flush() { }
  setServerURL() { }
  setLoggingEnabled() { }
  optInTracking() { }
  optOutTracking() { }
}

module.exports = { Mixpanel }
