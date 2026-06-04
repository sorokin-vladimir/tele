Output demo.gif

Require tele

Set Shell "bash"
Set FontSize 13
Set Width 1440
Set Height 720
Set Theme "Catppuccin Mocha"
Set Framerate 30
Set TypingSpeed 70ms
Set WindowBar "Colorful"
Set Padding 12
Set Margin 12
Set MarginFill "#6B50FF"
Set BorderRadius 8
Set CursorBlink false

# Launch tele (session must already exist at ~/.config/tele/session.json)
Type "tele"
Sleep 1000ms
Enter

# Wait for Telegram connection and initial render
Sleep 4s

# ── Chat list navigation ──────────────────────────────────────────────────────
Sleep 500ms
Type "j"
Sleep 260ms
Type "j"
Sleep 260ms
Type "j"
Sleep 260ms
Type "j"
Sleep 260ms
Type "j"
Sleep 900ms
Type "k"
Sleep 280ms
Type "k"
Sleep 700ms

# Scroll half-pages down/up
Ctrl+D
Sleep 550ms
Ctrl+D
Sleep 500ms
Ctrl+U
Sleep 700ms


# ── Search ──────────────────────────────────────────────
Type "/"
Sleep 300ms
Type "Sorokin"
Sleep 1000ms
Enter
Sleep 2500ms

# ── Compose and send a message (deliberate typo — will fix in edit step) ──────
Type "i"
Sleep 400ms
Type "Hello wrold! tele is great"
Sleep 600ms
Enter
Sleep 900ms
Escape
Sleep 400ms

# ── Edit the sent message — fix the typos ────────────────────────────────────
Space 1
Sleep 500ms
Down 2
Sleep 900ms
Enter
Sleep 500ms
Backspace 13
Sleep 200ms
Type "`tele` — keyboard-driven Telegram client for the terminal"
Sleep 500ms
Enter
Sleep 200ms
Escape
Sleep 1000ms

# ── Scroll up through history, set a reaction ────────────────────────────────
Type "k"
Sleep 200ms
Type "k"
Sleep 200ms
Type "k"
Sleep 200ms
Type "k"
Sleep 200ms
Type "k"
Sleep 700ms
Space 1
Sleep 500ms
Type "t"
Right 3
Sleep 300ms
Enter
Sleep 1200ms

# ── Scroll up more, reply to a message ───────────────────────────────────────
Type "k"
Sleep 260ms
Type "k"
Sleep 260ms
Type "k"
Sleep 700ms
Type "r"
Sleep 500ms
Type "nice one"
Sleep 500ms
Enter
Sleep 1000ms

# ── Back to chat list, walk through chats ────────────────────────────────────
Escape 2
Sleep 400ms
Type "j"
Sleep 280ms
Type "j"
Sleep 280ms
Type "k"
Sleep 280ms
Type "j"
Sleep 280ms
Type "k"
Sleep 600ms

# ── Folder sidebar ───────────────────────────────────────────────────────────
Type "0"
Sleep 700ms
Type "j"
Sleep 300ms
Type "j"
Sleep 500ms
Enter
Sleep 1000ms

# ── Jump to the list's bottom (Shift+D), confirm ───────────────────────────────────
# Type "D"
Down 40
Sleep 800ms
Enter
Sleep 1500ms

# ── Cleanup demo chat ───────────────────────────────────
Hide
Type "1"
Type "/"
Type "Sorokin"
Enter
Sleep 100ms
Space 1
Type "d"
Enter
Space 1
Type "d"
Enter
Show

# ── Quit ──────────────────────────────────────────────────────────────────────
Type "q"
Sleep 500ms
