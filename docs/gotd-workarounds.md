# gotd workarounds

tele works around defects in [gotd](https://github.com/gotd/td), most of them in
its updates manager. Each workaround is meant to be deleted once gotd fixes the
defect upstream. This page lists all of them in one place, so that bumping the
gotd dependency is also the moment each one is reconsidered.

The comments next to each workaround explain how it works. This page is the
index: where each one lives, what retires it, and whether that has happened.

## Why these exist

gotd's updates manager keeps the account's position in the update sequence
right, and nothing more. When a gap opens, it buffers what arrives and discards
whatever it cannot fit back into the sequence, and the position moves on anyway.
A catch-up that ends at the current position does not prove that every change
along the way was applied.

So tele guarantees delivery itself. An update may reach the store more than
once, and the store applies it according to the position it carries, not the
order it arrived in, so a second copy changes nothing. Each workaround below
closes one place where gotd lets an update fall through.

## Before bumping gotd

1. Check the upstream status of every entry below. An entry is retired only when
   its fix is in a release, not just merged.
2. Diff `telegram/updates` and `mtproto/read.go` between the current and the new
   version. If neither changed, no entry below changed either.
3. Run the tripwire tests: `go test ./internal/tg/ -run 'TestStand_Without'`.
   Each one reproduces its defect against the real manager and asserts that the
   defect is still there. A failing tripwire means upstream has fixed it.
4. Update the status column here, even when nothing retires.

## Workarounds

| Workaround | Where | Issue | Retired by | Status (gotd v0.162.0) |
|---|---|---|---|---|
| Channel difference cooldown stripped | `internal/tg/channel_diff.go` | [#266](https://github.com/sorokin-vladimir/tele/issues/266) | [gotd/td#1852](https://github.com/gotd/td/issues/1852) | open, no fix yet |
| Common-state difference delivered directly | `internal/tg/common_diff.go` | [#267](https://github.com/sorokin-vladimir/tele/issues/267) | [gotd/td#1854](https://github.com/gotd/td/pull/1854), for [gotd/td#1853](https://github.com/gotd/td/issues/1853) | PR open, not merged |
| Outbox reads taken off the wire | `internal/tg/outbox_hook.go` | [#68](https://github.com/sorokin-vladimir/tele/issues/68) | [gotd/td#1853](https://github.com/gotd/td/issues/1853) | open, and the fix may not cover our case |
| Clock skew read from the log | `internal/tg/clock_skew.go` | [#277](https://github.com/sorokin-vladimir/tele/issues/277) | [gotd/td#1856](https://github.com/gotd/td/issues/1856) | open, no fix yet |

### Channel difference cooldown stripped

Telegram attaches a `timeout` to a channel difference: 300 seconds for a busy
supergroup. gotd treats it as a ban on asking for that channel's difference
again, and while the ban holds it discards every update the channel receives. A
gap that opens inside the window freezes the chat for up to five minutes, and
every catch-up arms the ban again.

`channelDiffAPI` wraps the API the manager talks to and removes the `timeout`
field from every `updates.getChannelDifference` response, so the manager closes
a gap on its own 500 ms gap timer. `maxChannelDiffConcurrency` bounds how many
channel differences run at once. It stays when the wrapper goes.

- Tripwire: `TestStand_WithoutTheWrapperAChannelGapWaitsOutTheCooldown`
  (`internal/tg/channel_gap_stand_test.go`)
- On retirement: delete `channelDiffAPI`, its tests, the stand, and this entry;
  the manager takes the raw API again in `internal/tg/client.go`.

### Common-state difference delivered directly

When a gap opens in the account's common update sequence, the manager applies
the recovered difference through its own sequence and then jumps the position
to where the difference ends. Anything still buffered behind the gap is now
behind the position and gets discarded as outdated. Telegram also condenses a
difference: a message edited thirty times comes back as one edit, so losing it
freezes the message at whatever it said before the gap.

`commonDiffAPI` wraps the API the manager talks to and hands the updates in a
recovered difference to the dispatcher itself, before returning the difference
to the manager unchanged. The manager's own copy then arrives second and changes
nothing.

- Tripwire: `TestStand_WithoutTheWrapperTheFinalTextIsLost`
  (`internal/tg/common_gap_stand_test.go`)
- On retirement: delete `commonDiffAPI`, its tests, the stand, and this entry.

### Outbox reads taken off the wire

`UpdateReadHistoryOutbox` and `UpdateReadChannelOutbox` say that the other side
has read our messages. When one arrives while a gap is open, the manager
buffers it, and once the catch-up moves the position past it, the buffer is
discarded. Our messages then stay unread on screen.

`outboxHook` sits between the raw connection and the manager, and takes these
two updates out of every envelope before the manager sees it. It also logs the
type of every arriving update, which is the only record of what the server
actually pushed.

This is the same buffer-discard mechanism as the entry above, reported upstream
as gotd/td#1853, which gotd/td#1854 is meant to fix. The fix dispatches the
updates a difference replays, and that may not be enough here: the tripwire
below drops a receipt the difference never mentions at all. We asked upstream
which of the two it is
([comment](https://github.com/gotd/td/issues/1853#issuecomment-5750104496)). If
a difference is authoritative for its whole range, dropping the receipt is
correct and this workaround stays on our side regardless of the fix.

- Tripwire: `TestStand_WithoutTheHookAnOutboxReadBehindAGapIsLost`
  (`internal/tg/outbox_gap_stand_test.go`)
- On retirement: remove the extraction from `outboxHook`. Keep the hook itself
  if the arrival log is still wanted.

### Clock skew read from the log

This one is not in the updates manager but in the connection underneath it.
gotd checks the time in the id of every message it receives against the local
clock, and drops the message if it is more than 300 seconds old or 30 seconds
early. It keeps no difference between the local clock and Telegram's, which the
protocol asks a client to do, so a clock that is off by more than that drops
everything Telegram sends. Nothing is answered, no error is raised, and the
connection looks alive. A clock only 30 seconds behind is enough.

The only trace is a Warn entry per dropped message, `Ignoring rejected
message`, whose error names the id of the message. `watchClockSkew` wraps the
logger handed to gotd, reads the server's time out of that id and reports the
skew, so the app can say what is wrong instead of hanging. A successful reply
or an arriving update ends it.

This depends on the wording of a log entry, which gotd may change in any
release without notice, so the tripwire checks the wording against gotd itself.

- Tripwire: `TestStand_WithoutCalibrationASkewedClockHangs`
  (`internal/tg/clock_skew_stand_test.go`). It fails if gotd connects despite
  the skew (the defect is fixed) or if the entry can no longer be read (the
  watch has gone blind).
- On retirement: if gotd corrects for the skew, delete `clock_skew.go`, its
  tests, the stand and this entry, and the app no longer needs to say anything.
  If gotd instead reports the skew as an error, read it from there and drop the
  log watch.
