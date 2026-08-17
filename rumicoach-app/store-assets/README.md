# Store assets

Source material for the Google Play (and later App Store) listings. Everything here is
generated, not hand-drawn — regenerate rather than retouch, then replace with real design
work when there is time.

## What is here

```
play/screenshots/phone/   1080x1920 PNG, six screens
play/graphics/            app-icon-512.png, feature-graphic.png (1024x500)

tools/capture-screenshots.mjs   regenerates the screenshots
tools/feature-graphic.html      the feature graphic, as a page to screenshot
```

## Why 1080x1920 and not a tall-phone aspect

Play rejects a screenshot whose long side is more than twice its short side. The obvious
choice — a modern 9:19.5 phone viewport — is 2.17:1 and gets refused. These are captured at
360x640 CSS with a device scale factor of 3, which lands on exactly 16:9.

## Regenerating the screenshots

They come from the real app: the Expo web build running against the e2e mock server, so the
fixture user's data is stable and no real account is involved.

```bash
# 1. mock API
cd e2e/mock-server && MOCK_DEBUG=1 bun server.js

# 2. the app, pointed at it (the expo-web-mock entry in .claude/launch.json)
npx expo start --web --port 8084

# 3. capture (needs playwright available on the machine, not in this repo)
node store-assets/tools/capture-screenshots.mjs store-assets/play/screenshots/phone
```

The login step is the fragile part: the email form has no submit-on-Enter, and the six-digit
code auto-submits on the last digit, so the script clicks CONTINUE for the first and tolerates
a missing button on the second.

## Regenerating the feature graphic

`tools/feature-graphic.html` is rendered at exactly 1024x500. Background art is
`assets/theme/mountain_lake.jpg` (800x533), which upscales ~1.3x — the blur is there to make
that softness read as depth of field rather than as a low-resolution asset.

## Uploading

These have to be attached by hand in Play Console → Grow users → Store presence → Store
listings → Graphics. The console's uploader opens a native file dialog, and its CSP blocks
loading images from anywhere else, so there is no scripted path short of the Play Developer
API (`edits.images.upload`), which needs a service-account token this repo does not carry
locally.
