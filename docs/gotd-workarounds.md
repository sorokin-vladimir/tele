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
2. Diff `telegram/updates`, `mtproto/read.go` and `telegram/session.go` between the current and the new version.
   If neither changed, no entry below changed either.
3. Run the tripwire tests: `go test ./internal/tg/ -run 'TestStand_Without'`.
   Each one reproduces its defect against the real manager and asserts that the defect is still there.
   A failing tripwire means upstream has fixed it.
4. Update the status column here, even when nothing retires.

## Workarounds

| Workaround | Where | Issue | Retired by | Status (gotd v0.166.1) |
|---|---|---|---|---|
| Common-state difference delivered directly | `internal/tg/common_diff.go` | [#267](https://github.com/sorokin-vladimir/tele/issues/267) | [gotd/td#1941](https://github.com/gotd/td/pull/1941), for [gotd/td#1853](https://github.com/gotd/td/issues/1853) | fixed in v0.164.0, tripwire removed; the workaround is to go |
| Outbox reads taken off the wire | `internal/tg/outbox_hook.go` | [#68](https://github.com/sorokin-vladimir/tele/issues/68) | [gotd/td#1853](https://github.com/gotd/td/issues/1853) | closed, but the tripwire still holds: the fix does not cover our case |
| Clock skew read from the log | `internal/tg/clock_skew.go` | [#277](https://github.com/sorokin-vladimir/tele/issues/277) | [gotd/td#1968](https://github.com/gotd/td/pull/1968), for [gotd/td#1856](https://github.com/gotd/td/issues/1856) | issue open, but v0.165.0 corrects for the skew; tripwire removed, the workaround is to go |
| Saved DC addresses handed to the client | `internal/tg/session.go` | [#312](https://github.com/sorokin-vladimir/tele/issues/312) | [gotd/td#1871](https://github.com/gotd/td/issues/1871) | open, no fix yet |

### Common-state difference delivered directly

When a gap opens in the account's common update sequence, the manager applies the recovered difference through its own sequence and then jumps the position to where the difference ends.
Anything still buffered behind the gap is now behind the position and gets discarded as outdated.
Telegram also condenses a difference: a message edited thirty times comes back as one edit, so losing it freezes the message at whatever it said before the gap.

`commonDiffAPI` wraps the API the manager talks to and hands the updates in a recovered difference to the dispatcher itself, before returning the difference to the manager unchanged.
The manager's own copy then arrives second and changes nothing.

- Tripwire: `TestStand_WithoutTheWrapperTheFinalTextIsLost` fired on v0.166.1 and was removed.
- On retirement: delete `commonDiffAPI`, its tests, the stand, and this entry.

### Outbox reads taken off the wire

`UpdateReadHistoryOutbox` and `UpdateReadChannelOutbox` say that the other side has read our messages.
When one arrives while a gap is open, the manager buffers it, and once the catch-up moves the position past it, the buffer is discarded.
Our messages then stay unread on screen.

`outboxHook` sits between the raw connection and the manager, and takes these two updates out of every envelope before the manager sees it.
It also logs the type of every arriving update, which is the only record of what the server actually pushed.

This is the same buffer-discard mechanism as the entry above, reported upstream as gotd/td#1853.
gotd/td#1941 closed it in v0.164.0 by dispatching the updates a difference replays, and that is not enough here: the tripwire below drops a receipt the difference never mentions at all, and it still holds on v0.166.1.
We asked upstream whether a difference is authoritative for its whole range ([comment](https://github.com/gotd/td/issues/1853#issuecomment-5750104496)) and got no answer.
If it is, dropping the receipt is correct and this workaround stays on our side for good.

- Tripwire: `TestStand_WithoutTheHookAnOutboxReadBehindAGapIsLost` (`internal/tg/outbox_gap_stand_test.go`)
- On retirement: remove the extraction from `outboxHook`.
  Keep the hook itself if the arrival log is still wanted.

### Clock skew read from the log

This one is not in the updates manager but in the connection underneath it.
gotd checks the time in the id of every message it receives against the local clock, and drops the message if it is more than 300 seconds old or 30 seconds early.
It keeps no difference between the local clock and Telegram's, which the protocol asks a client to do, so a clock that is off by more than that drops everything Telegram sends.
Nothing is answered, no error is raised, and the connection looks alive.
A clock only 30 seconds behind is enough.

The only trace is a Warn entry per dropped message, `Ignoring rejected message`, whose error names the id of the message.
`watchClockSkew` wraps the logger handed to gotd, reads the server's time out of that id and reports the skew, so the app can say what is wrong instead of hanging.
A successful reply or an arriving update ends it.

This depends on the wording of a log entry, which gotd may change in any release without notice, so the tripwire checks the wording against gotd itself.

- Tripwire: `TestStand_WithoutCalibrationASkewedClockHangs` fired on v0.166.1 and was removed: gotd connected despite a clock ten minutes ahead, and wrote no entry to read.
  v0.165.0 keeps the difference between the clocks, as TDLib does (gotd/td#1968), although gotd/td#1856 is still open.
- On retirement: if gotd corrects for the skew, delete `clock_skew.go`, its tests, the stand and this entry, and the app no longer needs to say anything.
  If gotd instead reports the skew as an error, read it from there and drop the log watch.

### Saved DC addresses handed to the client

This one is not in the updates manager either, but in how gotd restores a session.
Every time the client connects, gotd writes the config Telegram sent into the session file, data-centre addresses included.
When it restores the session at the next start, it reads the data centre, the key and the salt back, but not the config, so it dials the static list compiled into gotd (`telegram/dcs/prod.go`).
That list can name addresses Telegram no longer serves, or that a network cannot reach.
A login works, because by the time it moves to the account's data centre gotd holds the live config, and every start after it dials the static addresses and retries them forever.

`savedDCList` reads the addresses out of the session file and `Connect` hands them to gotd as its starting list.
With no session, or one that saved no addresses, the list stays gotd's own.

- Tripwire: `TestStand_WithoutTheSavedListASessionDialsTheStaticAddresses` (`internal/tg/session_dcs_stand_test.go`).
  It fails once gotd dials the saved address itself.
- On retirement: delete `savedDCList`, the `DCList` option in `internal/tg/client.go`, the stand and this entry; keep `TestConnect_DialsTheAddressesTheSessionSaved`, which holds the behaviour whoever provides it.
