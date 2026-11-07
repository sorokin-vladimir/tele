tele for AI agents

tele is a Telegram client. Its commands ask the tele that holds the person's
account, a window they have open or the service, and act on that account on
their behalf. This is how to use them.

Start by checking you can reach the account:

    tele chats --json --limit 1

Exit 0 means you can go on. Any other code is in the table at the end: stop
and do what it says.

NAMING A CHAT

A chat is named by its id, its @username, or me (the person's own Saved
Messages), the same in every command. To find a chat the person names by its
title, list them with `tele chats --json`, match the title yourself, and use
the `id`. Titles change and repeat, so the id is what you pass on.

READING

Pass --json every time. A list comes as JSON Lines: one object per line,
oldest first where order means time.

    tele chats --json [--unread] [--archived | --folder ID] [--limit N]
    tele topics CHAT --json
    tele show CHAT --json [--topic ID] [--limit N] [--before ID]

A chat with "forum": true keeps its messages in topics: list them with
`tele topics`, then pass --topic to `tele show`. To read further back, pass
--before the oldest id you already have. Read what the task needs, a page at a
time.

Reading changes nothing in the account. Mark a chat read only when the person
asks for it: `tele show CHAT --mark-read` marks up to the last message printed.

SENDING

Send a message only when the person asked for that message to be sent, with
the text they approved.

    tele send CHAT --ref KEY --json "text"
    printf '%s' "$text" | tele send CHAT --ref KEY --json

Give every send a --ref you make up and keep, such as a short random string.
Running the same send again with the same --ref never sends a second message:
it waits for the first or says it went. That is how you retry safely.

Markup is read as in tele's composer: **bold**, `code`. Pass --plain to send
the text exactly as it is. A forum is written to in its General topic unless
--topic names another.

EXIT CODES

| Code | Meaning                                               | What to do |
| ---- | ----------------------------------------------------- | ---------- |
| 0    | Done                                                  | Go on |
| 1    | Something else went wrong; stderr says what          | Report stderr to the person |
| 2    | The command line was wrong                            | Fix the call; `tele COMMAND --help` lists the flags |
| 3    | No tele is running that holds the account             | Tell the person: they open tele, or install the service with `tele daemon install` |
| 4    | The account is not logged in                          | Tell the person to log in, in a tele window |
| 5    | No such chat, topic or folder                         | Look the name up again with `tele chats --json` or `tele topics` |
| 6    | The message will not go; stderr says why              | Report the reason. The message waits in the chat for the person to retry or discard it |
| 7    | The person limited what commands may do               | Stop and tell the person what you were asked to do. Their setting, cli.account_commands, is theirs to change |
| 75   | Still queued, Telegram asked to wait, or tele was lost | The message goes on its own. To confirm later, run the same send with the same --ref |

`tele COMMAND --help` lists every flag. The full contract, with every JSON
field, is docs/cli.md: https://github.com/sorokin-vladimir/tele/blob/main/docs/cli.md
