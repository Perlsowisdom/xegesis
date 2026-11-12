# Xegesis privacy policy

Last updated: 12 November 2026

Xegesis is a Bible study app for Windows. This page says what it does with your information. The short
version: **Xegesis collects nothing about you.** It has no accounts, no analytics and no tracking, and the
developer receives no data from your use of it.

## What stays on your computer

Everything you create or add (notes, sermons, highlights, lists, reading plans, books, Bible modules and
settings) is stored on your own computer, in the Xegesis folder of your user profile. Xegesis does not
upload it. Passwords and keys you give it (for example an API key for a Bible service) are kept in the
Windows credential store, not in Xegesis's files. Windows disk encryption protects this data; the app
recommends turning it on.

## When Xegesis uses the internet

Only the features below can, and **each is off until you turn it on**. Nothing is sent in the background
without your choice.

- **Bible catalogues and downloads.** When you turn catalogues on and ask to refresh or install, Xegesis
  fetches lists and files from public servers (CrossWire, eBible.org, STEPBible, OpenBible.info). Each
  request is a plain web request with no identifier, account or information about your library. The
  servers see your IP address and which files you ask for, as with any download.
- **Bibles online (API.Bible) and the Digital Bible Library.** If you enter your own key, Xegesis sends
  your requests for passages to that service with your key.
- **AI assistant.** Off until you choose a provider. If you choose an online provider (Claude, OpenAI,
  Gemini, OpenRouter or an address you give), what you ask, the conversation so far, and the passages and
  notes the assistant reads to answer are sent to that provider with your key, under that provider's own
  terms. Before your first message, Xegesis shows exactly what will be sent and where. If you choose a
  model on your own computer, nothing leaves it. Personal copies of books are never sent to an online
  provider.
- **Updates (direct download only).** The version of Xegesis from the project's website can look at its
  public GitHub releases page once a day, if you turn that on, and fetches a new installer only when you
  ask. The version from the Microsoft Store has no updater: the Store handles updates.

The file `docs/privacy.md` in the project lists the exact addresses and what each request contains.

## Crash and problem reports

Xegesis never sends reports by itself. Errors are written to a log on your computer. If a report feature
is offered, you will see exactly what it contains, can edit or remove parts of it, and choose whether to
send it. Reports never include your notes, document text, book titles, file paths, search terms or keys.

## Children

Xegesis does not knowingly collect information from anyone, children included.

## Changes

If this policy changes, the new version is published here with a new date, and the project's history
shows what changed.

## Contact

Questions or concerns: open an issue at https://github.com/Perlsowisdom/xegesis/issues.
