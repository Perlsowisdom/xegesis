# T35: Android app — listening, Android Auto and voice assistants

Brief for one agent (Opus), after the Android trial (T31) proves the approach in ADR 0008. Read
`AGENTS.md` first. Requested by the user on 28 October 2026.

Platform rules change often. Check every API and policy below against the current Android, Android
Auto, Google Assistant / Gemini and Samsung developer documentation before building. Note anything that
differs from this brief.

## 1. Listening (text to speech)

- Read Bibles and books aloud with the phone's own speech engine (Android `TextToSpeech`). It works
  offline and sends nothing anywhere (ADR 0009).
- The listening screen has play, pause, skip back and forward by verse or paragraph, speed, voice and
  language, and a sleep timer.
- The current verse or sentence is highlighted, and the place is remembered as the reading position.
- Playback is a proper media session: it keeps playing with the screen off, shows lock-screen and
  notification controls, and pauses for calls and navigation prompts.
- Licences: listening is reading, so it is allowed for every licence. Saving audio files counts as
  exporting and follows the export limit.

## 2. Android Auto

Android Auto lets media apps play audio but does not allow reading text on the car screen, so Xegesis
appears there as a listening app.
- Implement the media library service (Media3 `MediaLibraryService`), with a browsable tree: Continue
  listening, Bibles (book, then chapter), Reading plan for today, Recent books.
- Speech comes from the same text-to-speech playback as section 1.
- Follow the Android Auto media app quality guidelines, and test with the Desktop Head Unit.

## 3. Voice assistants (Google Assistant, Gemini, Bixby)

- **Media voice commands:** "Play John 3 on Xegesis" and "Read today's reading plan on Xegesis" go
  through the media session's play-from-search, which Assistant and Android Auto use. Parse the request
  with `verse.Parse` and the library search.
- **App actions and shortcuts:** "Open Romans 8 in Xegesis" and "Search Xegesis for grace" use Android
  shortcuts and capabilities (`shortcuts.xml` with built-in intents such as open app feature and get
  thing). Gemini's support for app actions and extensions must be checked for third-party apps at build
  time, and only what is genuinely supported implemented.
- **Bixby:** check whether Samsung still accepts new Bixby capsules. If not, rely on the standard
  Android media session and shortcuts, which Samsung phones also expose to their assistant, and say so.
- Every voice action maps to an existing command (`bible.read`, `book.open`, `search.query`, the
  reading-plan commands), so voice adds no features of its own.

## Rules

- Native Kotlin code stays in the thin Android shell (ADR 0008). The Go core is reached through
  `Run(command, json)`.
- Permissive dependencies only. AndroidX and Media3 are Apache-2.0.
- Privacy (ADR 0009): no analytics, and speech stays on the device. Assistants only receive what
  Android passes them.

## Done when

- A chapter plays aloud with the screen off and lock-screen controls work.
- The place is kept, and the sleep timer works.
- Xegesis appears in the Android Auto Desktop Head Unit and plays a Bible chapter chosen from its
  menus.
- "Play John 3 on Xegesis" works through Google Assistant on a test device, or the report explains
  precisely what the platform does not allow.
- `docs/android.md` documents what each assistant supports, with links checked on the day.
