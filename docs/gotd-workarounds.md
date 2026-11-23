# gotd workarounds

tele works around defects in [gotd](https://github.com/gotd/td), most of them in its updates manager.
Each workaround is meant to be deleted once gotd fixes the defect upstream.
This page lists all of them in one place, so that bumping the gotd dependency is also the moment each one is reconsidered.

The comments next to each workaround explain how it works.
This page is the index: where each one lives, what retires it, and whether that has happened.

## Why these exist

gotd's updates manager keeps the account's position in the update sequence right, and nothing more.
When a gap opens, it buffers what arrives and discards whatever it cannot fit back into the sequence, and the position moves on anyway.
A catch-up that ends at the current position does not prove that every change along the way was applied.

So tele guarantees delivery itself.
An update may reach the store more than once, and the store applies it according to the position it carries, not the order it arrived in, so a second copy changes nothing.
Each workaround below closes one place where gotd lets an update fall through.

## Before bumping gotd

1. Check the upstream status of every entry below.
   An entry is retired only when its fix is in a release, not just merged.
2. Diff `telegram/updates` and `telegram/session.go` between the current and the new version.
   If neither changed, no entry below changed either.
3. Run the tripwire tests: `go test ./internal/tg/ -run 'TestStand_Without'`.
   Each one reproduces its defect against the real manager and asserts that the defect is still there.
   A failing tripwire means upstream has fixed it.
4. Update the status column here, even when nothing retires.

## Workarounds

| Workaround | Where | Issue | Retired by | Status (gotd v0.166.1) |
|---|---|---|---|---|
| Outbox reads taken off the wire | `internal/tg/outbox_hook.go` | [#68](https://github.com/sorokin-vladimir/tele/issues/68) | [gotd/td#1853](https://github.com/gotd/td/issues/1853) | closed, but the tripwire still holds: the fix does not cover our case |
| Saved DC addresses handed to the client | `internal/tg/session.go` | [#312](https://github.com/sorokin-vladimir/tele/issues/312) | [gotd/td#1871](https://github.com/gotd/td/issues/1871) | open, no fix yet |

### Outbox reads taken off the wire

`UpdateReadHistoryOutbox` and `UpdateReadChannelOutbox` say that the other side has read our messages.
When one arrives while a gap is open, the manager buffers it, and once the catch-up moves the position past it, the buffer is discarded.
Our messages then stay unread on screen.

`outboxHook` sits between the raw connection and the manager, and takes these two updates out of every envelope before the manager sees it.
It also logs the type of every arriving update, which is the only record of what the server actually pushed.

The mechanism was reported upstream as gotd/td#1853: when a gap opens in the common sequence, the manager discards whatever is buffered behind it once the catch-up moves the position on.
gotd/td#1941 closed it in v0.164.0 by applying the updates a difference replays directly, and that is not enough here: the tripwire below drops a receipt the difference never mentions at all, and it still holds on v0.166.1.
We asked upstream whether a difference is authoritative for its whole range ([comment](https://github.com/gotd/td/issues/1853#issuecomment-5750104496)) and got no answer.
If it is, dropping the receipt is correct and this workaround stays on our side for good.

- Tripwire: `TestStand_WithoutTheHookAnOutboxReadBehindAGapIsLost` (`internal/tg/outbox_gap_stand_test.go`)
- On retirement: remove the extraction from `outboxHook`.
  Keep the hook itself if the arrival log is still wanted.

### Saved DC addresses handed to the client

This one is not in the updates manager but in how gotd restores a session.
Every time the client connects, gotd writes the config Telegram sent into the session file, data-centre addresses included.
When it restores the session at the next start, it reads the data centre, the key and the salt back, but not the config, so it dials the static list compiled into gotd (`telegram/dcs/prod.go`).
That list can name addresses Telegram no longer serves, or that a network cannot reach.
A login works, because by the time it moves to the account's data centre gotd holds the live config, and every start after it dials the static addresses and retries them forever.

`savedDCList` reads the addresses out of the session file and `Connect` hands them to gotd as its starting list.
With no session, or one that saved no addresses, the list stays gotd's own.

- Tripwire: `TestStand_WithoutTheSavedListASessionDialsTheStaticAddresses` (`internal/tg/session_dcs_stand_test.go`).
  It fails once gotd dials the saved address itself.
- On retirement: delete `savedDCList`, the `DCList` option in `internal/tg/client.go`, the stand and this entry; keep `TestConnect_DialsTheAddressesTheSessionSaved`, which holds the behaviour whoever provides it.
