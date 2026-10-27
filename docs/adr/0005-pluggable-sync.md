# 0005: Sync is a pluggable add-on

## Decision

Xegesis works fully offline with local version history from the start. Sync between devices is added in
milestone M3 as interchangeable adapters that exchange change records:

- **any-sync** (MIT) for users who self-host its server stack.
- **Folder sync** through a shared folder (Syncthing, Nextcloud and similar) for everyone else.

## Reasons

- A self-hosted any-sync network is about ten services (coordinator, three sync nodes, file node,
  consensus node, MongoDB, Redis, Garage): reasonable for self-hosters, too much for most users.
- any-sync supplies sync and signed change history, but not an object model; Xegesis needs its own anyway.

## Merge rules

- Changes to different fields merge automatically.
- When two devices change the same text field, both versions are kept for the user to choose.
