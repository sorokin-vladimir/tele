# 命令

> 本文译自 [commands.md](../commands.md)，对应提交 `f76c4c5`。译文与英文版不一致时，以英文版为准。本译文尚未经过母语者审校。如果发现翻译错误，欢迎[提交 issue](https://github.com/sorokin-vladimir/tele/issues)。

`tele` 的全部命令及其参数。命令的共同点（如何命名、如何指定聊天、以什么退出码结束、JSON 输出是什么样子）见 [cli.md](cli.md)。`tele help COMMAND` 或在任意命令后加 `--help`，可以直接从程序中看到同样的内容（英文）。

- [所有命令都接受的参数](#flags-every-command-takes)
- [`tele`](#tele)
- [`tele chats`](#tele-chats)
- [`tele topics`](#tele-topics)
- [`tele show`](#tele-show)
- [`tele send`](#tele-send)
- [`tele version`](#tele-version)
- [`tele theme check`](#tele-theme-check)
- [`tele theme dump`](#tele-theme-dump)
- [`tele daemon`](#tele-daemon)
- [`tele daemon status`](#tele-daemon-status)
- [`tele daemon install`](#tele-daemon-install)
- [`tele daemon uninstall`](#tele-daemon-uninstall)

<a id="flags-every-command-takes"></a>
## 所有命令都接受的参数

| 参数                    | 作用                                                                                 |
| ----------------------- | ------------------------------------------------------------------------------------ |
| `-c`, `--config <path>` | 要读取的配置文件，默认为 `~/.config/tele/config.yml`，Windows 上也是如此。它同时决定状态目录，从而决定命令针对的是哪个账号 |
| `-e`, `--debug`         | 以 debug 级别记录日志                                                                |
| `--trace`               | 额外记录能识别聊天和人的信息：对方 id、消息长度。切勿在共享或同步的文件系统上使用    |
| `-h`, `--help`          | 说明命令的作用和它接受的参数                                                         |

如果配置文件还不存在，会先用默认值写入一份。日志是 `$XDG_STATE_HOME/tele` 中的 `tele.log`，未设置该变量时位于 `~/.local/state/tele`，与账号无关。

<a id="tele"></a>
## `tele`

```console
tele [-c PATH]
```

打开一个显示账号的窗口。如果没有其他 `tele` 持有该账号，就由这个窗口持有，直到窗口关闭；如果已有 `tele` 持有（某个窗口、`tele daemon` 或服务），这个窗口会连接到它并共用它的连接。关闭一个持有账号、且还有其他窗口连接着的窗口时，会先询问，然后把那些窗口一起关闭。

持有账号的 `tele` 第一次在可以安装服务的机器上登录时，会询问一次是否安装服务。参见[无窗口运行](configuration.md#running-without-a-window)。

<a id="tele-chats"></a>
## `tele chats`

```console
tele chats [--unread] [--archived | --folder ID] [--limit N] [--json]
```

按聊天列表中的顺序列出聊天，每个聊天一行：聊天的 id、@username、标题和标记（未读、提及、回应、手动标为未读、静音，论坛还会显示有未读的话题数）。没有用户名的聊天，这一列留空；列表中所有聊天都没有用户名时，整列省略。用户名从不截断，因此可以原样粘贴到命令中。标记都从同一列开始，即最长的标题之后；标题宽于 `cli.name_width` 列（默认 20）时会被截断。不加参数时列出主列表，不含已归档的聊天。

| 参数            | 作用                                                                            |
| --------------- | ------------------------------------------------------------------------------- |
| `--unread`      | 只列出有未读内容的聊天，包括手动标为未读的                                      |
| `--archived`    | 列出已归档的聊天，而不是主列表                                                  |
| `--folder <id>` | 列出你的某个文件夹中的聊天，与聊天列表中显示的一致。没有这个文件夹时以退出码 5 结束，并列出已有的文件夹 |
| `--limit <n>`   | 最多列出这么多个，取其他参数筛选后的前几个。不加此参数时全部列出               |
| `--json`        | 每个聊天输出一个 JSON 对象，见 [cli.md](cli.md#tele-chats---json)               |

<a id="tele-topics"></a>
## `tele topics`

```console
tele topics CHAT [--limit N] [--json]
```

按话题列表中的顺序列出论坛的话题，每个话题一行：话题的 id、标题和标记，标题与 `tele chats` 中一样按 `cli.name_width` 截断。聊天不是论坛时以退出码 5 结束。`tele show --topic` 接受的就是这个 id。

| 参数          | 作用                                                                  |
| ------------- | --------------------------------------------------------------------- |
| `--limit <n>` | 最多列出这么多个，取列表的前几个。不加此参数时全部列出                |
| `--json`      | 每个话题输出一个 JSON 对象，见 [cli.md](cli.md#tele-topics---json)    |

<a id="tele-show"></a>
## `tele show`

```console
tele show CHAT [--topic ID] [--limit N] [--before ID] [--mark-read] [--json]
```

按从旧到新的顺序打印聊天的最新消息：每条消息一行，包含时间、发送者和文本；日期变化处有一行日期；回复下方有一行说明它回复的是哪条消息。所有消息的文本都从同一列开始，即最长的发送者名字之后；名字宽于 `cli.name_width` 列（默认 20）时会被截断。媒体只显示名称，从不下载：`[photo]`、`[voice 0:12]`、`[file report.pdf]`。本地历史中没有的消息会从 Telegram 获取，与在窗口中向上滚动时一样。

除非明确要求，`show` 不做任何改动：加上 `--mark-read` 时，它把聊天标为已读，直到打印出的最后一条消息为止，不会更多。

论坛的消息都在它的话题里：用 `--topic` 指定一个话题，否则 `show` 以退出码 5 结束，并提示使用 `tele topics`。

| 参数            | 作用                                                                            |
| --------------- | ------------------------------------------------------------------------------- |
| `--topic <id>`  | 要显示的论坛话题                                                                |
| `--limit <n>`   | 显示多少条消息，默认 20，最多 200                                               |
| `--before <id>` | 显示比这条消息更早的消息，用于向前翻阅历史                                      |
| `--mark-read`   | 把聊天（或话题）标为已读，直到打印出的最后一条消息为止                          |
| `--json`        | 每条消息输出一个 JSON 对象，见 [cli.md](cli.md#tele-show---json)                |

<a id="tele-send"></a>
## `tele send`

```console
tele send CHAT [TEXT...] [--topic ID] [--plain] [--ref KEY] [--timeout D] [--no-wait] [--json]
echo "the build is green" | tele send @team
```

发送一条消息：内容是传入的单词，以空格连接；没有传入时，则是从 stdin 读到的内容。标记的解析方式与输入框相同（`**bold**`、`` `code` `` 等）；`--plain` 按原样发送文本。发往论坛时，除非用 `--topic` 指定其他话题，消息会进入 General 话题。

消息先进入队列，和在窗口中输入的消息一样，所以持有账号的 `tele` 重启后它仍然存在。然后 `send` 等待它发出，并打印它成为的那条消息的 id：

- 如果消息不会发出，以退出码 6 结束，并说明原因。消息留在聊天中，等人在窗口里重试或丢弃。
- 如果时间到了消息仍在队列中，或者失去了所询问的 `tele`，以退出码 75 结束。消息会自行发出。

`--ref` 为这次发送命名。用同一个 `--ref` 再次运行同一个 `tele send` 不会发出第二条消息：它会等待第一条，或者说明它已经发出。在 75 之后或丢失回应之后重试的脚本应使用它。ref 在其消息发出后会被记住 30 天。

| 参数              | 作用                                                                            |
| ----------------- | ------------------------------------------------------------------------------- |
| `--topic <id>`    | 发往论坛的哪个话题；不加时为 General                                            |
| `--plain`         | 按原样发送文本，不解析其中的标记                                                |
| `--ref <key>`     | 为这次发送命名，这样再次运行同一命令时不会发送两次                              |
| `--timeout <d>`   | 等待消息发出的时长，默认 `30s`                                                  |
| `--no-wait`       | 消息进入队列后立即返回，给出它的 ref 和所处状态                                 |
| `--json`          | 以一个 JSON 对象输出回答，见 [cli.md](cli.md#tele-send---json)                  |

<a id="tele-version"></a>
## `tele version`

```console
tele version
```

只输出版本号，便于脚本读取：`2.0.0`、`2.0.0-beta.11`，或者对于没有版本号的源码构建输出 `dev`。

`tele --version` 作用相同。保留它是为了那些在测试中调用它的包管理器，帮助中不会列出。

<a id="tele-theme-check"></a>
## `tele theme check`

```console
tele theme check [NAME]
```

说明每个槽位（深色和浅色）最终解析成了哪个主题、它沿着哪条基础主题链继承而来，以及有多少 token 来自何处，包括那些回退到内置主题的 token。如果主题设定了自己的画布，还会列出在该画布上难以辨认的 token。指定 `NAME` 时，只检查这一个主题：槽位（`dark`、`light`）或按名称指定的主题（`nord`、`mine`）。关于主题文件的警告列在最后。

`tele --theme-check[=NAME]` 作用相同，帮助中不会列出。参见 [themes.md](themes.md#when-something-looks-wrong)。

<a id="tele-theme-dump"></a>
## `tele theme dump`

```console
tele theme dump [NAME] > ~/.config/tele/themes/mine.yml
```

把一个主题输出为完整的主题文件，每个 token 都有值，可以直接编辑：不指定 `NAME` 时输出深色槽位的主题，否则输出 `NAME` 指定的槽位（`dark`、`light`）或主题。stdout 中只有文件内容，因此可以直接重定向；出错时信息输出到 stderr，退出码为 1。

`tele --theme-dump[=NAME]` 作用相同，帮助中不会列出。参见 [themes.md](themes.md)。

<a id="tele-daemon"></a>
## `tele daemon`

```console
tele daemon [--service]
```

在没有窗口的情况下持有账号，直到被停止（Ctrl+C，或服务管理器发送的信号）。它保持连接、发送桌面通知，窗口和命令可以像连接到窗口一样连接到它。它只会持有账号：如果已有其他 `tele` 持有该账号，它以退出码 1 结束，并给出那个 `tele` 的版本和进程 id。

| 参数        | 作用                                                                                               |
| ----------- | -------------------------------------------------------------------------------------------------- |
| `--service` | 作为服务运行：由服务管理器启动，不需要手动输入。以这种方式启动的 `tele` 在账号被其他 `tele` 持有时不会退出，而是等待，并在那个 `tele` 退出后接管。当更新版本的 `tele` 要求它重启时，它以退出码 75 结束，以便服务管理器启动当前安装的版本 |

<a id="tele-daemon-status"></a>
## `tele daemon status`

```console
tele daemon status [--json]
```

说明持有账号的 `tele`（包括窗口）的运行状况：它的版本与当前版本的对比，是什么让它保持运行（窗口、`tele daemon` 或服务）以及从何时开始，连接到它的窗口和其他命令的数量，账号所处的状态，与 Telegram 的连接是否正常，最近一次更新何时到达，有多少消息等待发送，Telegram 要求的任何等待，以及命令是否受到 [`cli.account_commands`](cli.md#keeping-commands-out-of-your-chats) 的限制。如果已安装的服务运行的命令已不存在，或者版本与当前版本不同，它也会发出警告。

有 `tele` 回应时以退出码 0 结束，没有任何回应时以 3 结束。

| 参数     | 作用                                                                                             |
| -------- | ------------------------------------------------------------------------------------------------ |
| `--json` | 以单个 JSON 对象输出结果，格式见 [cli.md](cli.md#tele-daemon-status---json)。没有回应时不输出任何内容 |

<a id="tele-daemon-install"></a>
## `tele daemon install`

```console
tele daemon install [--path COMMAND]
```

让操作系统以服务的形式保持 `tele` 运行，对应传入的配置：macOS 上是 LaunchAgent，Linux 上是 systemd 用户单元，Windows 上是任务计划程序中的任务。服务在每次登录系统时启动，退出后会再次启动，并且安装后立即启动。如果此时有 `tele` 持有账号，服务会等待，并在那个 `tele` 退出后接管。

服务按 `PATH` 找到的方式运行 `tele`，这样升级会替换它启动的程序。如果 `PATH` 中的 `tele` 不是正在执行该命令的那个，`install` 会拒绝并给出两者。重新安装会替换服务并重新启动它。每个配置文件对应一个服务。在没有服务管理器可用的系统上，它会说明如何在你自己的会话中启动 `tele daemon`，并以退出码 1 结束。

| 参数               | 作用                                                                                 |
| ------------------ | ------------------------------------------------------------------------------------ |
| `--path <command>` | 服务运行的命令，用来代替按 `PATH` 找到的 `tele`。使用 Scoop 时需要它，因为 Scoop 放在 `PATH` 中的 `tele` 是一个 shim |

参见[作为服务运行](configuration.md#as-a-service)。

<a id="tele-daemon-uninstall"></a>
## `tele daemon uninstall`

```console
tele daemon uninstall
```

停止并删除传入配置对应的服务。连接到该服务的窗口会收到服务已被卸载的通知，并停止等待它。状态和配置保持原样。如果没有安装服务，它会如实说明，并以退出码 0 结束。
