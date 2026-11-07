# 配置

> 本文译自 [configuration.md](../configuration.md)，对应提交 `a8ea9ea`。译文与英文版不一致时，以英文版为准。本译文尚未经过母语者审校。如果发现翻译错误，欢迎[提交 issue](https://github.com/sorokin-vladimir/tele/issues)。

`tele` 使用一个 YAML 文件，首次运行时创建：

```text
~/.config/tele/config.yml
```

要使用其他文件，传入 `--config <path>`。

<a id="the-settings-overlay"></a>
## 设置面板

按 `,` 打开设置面板：其中列出 `tele` 的所有设置项，分组和顺序与配置文件一致，所以面板里看到的每一行都能在文件中的相同路径找到。面板显示每项的当前值，标出没有人选择过的项（`[default]`），并说明修改何时生效：立即生效、下次 `tele` 执行该操作时（`[next]`）、重启后（`[restart]`），或者持有连接的 `tele` 重启后（`[host restart]`）。快捷键也列在这里，包括你的配置改了什么、原来是什么。

在面板中做的修改会直接写入 `config.yml`，保留你的注释、空行以及文件中 `tele` 不认识的任何内容。在编辑器里改文件和在面板里改是一回事：两者最后都会重新读取文件，所以二者永远不会不一致。`enter` 修改光标所在的设置项，`r` 恢复默认值，`esc` 关闭面板。

有四个设置项只显示、不能在面板中修改：这是哪个账号，以及它的状态存放在哪里。修改它们意味着移动文件并重新打开会话，所以需要有意识地编辑文件，而不是按一个键。

有些设置项属于持有 Telegram 连接的 `tele`：应用密钥、状态目录、代理、缓存大小、每次加载多少历史消息，以及桌面通知。其余的属于你正在看的窗口：主题、快捷键、弹出提示、图片的绘制方式。每个 `tele` 从自己的配置文件读取属于自己的那一类。连接到其他 `tele` 的窗口（见[状态目录](#state-directory)）显示的第一类设置是那个 `tele` 所持有的值，在窗口中做的修改会发送给它：它把修改写入自己的配置文件（可能不是窗口的那个文件）并应用。窗口自己的设置照常写入窗口的文件。

从窗口重新加载配置会同时重新加载两者：窗口重新读取自己的文件，持有连接的 `tele` 读取它自己的文件。任何一方发现的问题都在同一条提示中说明；如果某一方不接受自己的文件，它会如实说明，而不会撤销另一方已接受的内容。

<a id="configyml"></a>
## config.yml

```yaml
# proxy: # how tele reaches Telegram; absent means auto
#   type: mtproto # auto | direct | mtproto | socks5
#   server: 127.0.0.1
#   port: 1443
#   secret: ee... # mtproto only, hex or base64url
#   username: "" # socks5 only
#   password: "" # socks5 only

# state_dir: ~/.local/state/tele # session, local database and instance lock

ui:
  history_limit: 50 # messages fetched per chat on open
  confirm_quit: false # ask before q/ctrl+c/ctrl+q closes tele
  # theme: # omit for the built-in tele-dark / tele-light
  #   dark: my-dark # ~/.config/tele/themes/my-dark.yml
  #   light: my-light

  notifications:
    desktop: true # hand it to the OS notification service
    toast: true # draw it in a corner of tele's own window
    preview: true # set false to send the sender's name and no message text

  toasts:
    error_zone: bottom-right # bottom-right | top-right
    notify_zone: top-right # bottom-right | top-right
    max_visible: 3 # per corner; the rest are counted, not drawn

photos:
  mode: auto # auto | kitty | blocks - inline image renderer
  eager_full_quality: true # download full resolution in the background on chat open
  kitty_placement_cap: 16 # max inline images kept on the terminal at once
  max_long_side_px: 800 # cap a rendered image's long side; height also ≤ 2/3 pane
  disk_cache_size: 268435456 # 256 MB of fetched chat media kept between runs

avatars:
  disk_cache_size: 16777216 # 16 MB of people's pictures, budgeted separately

cli:
  name_width: 20 # columns a command gives a name or title before cutting it short
  account_commands: all # all | read | none: what commands may do with your chats

# keybindings: see keybindings.md
```

<a id="proxy"></a>
## 代理

`proxy` 决定 `tele` 如何连接 Telegram 的数据中心。每个连接都会经过它：消息、图片、语音、文件，无论来自哪个数据中心。这一节在持有连接的 `tele` 启动时读取，所以修改在 `[host restart]` 后生效。

`proxy.type` 指定连接方式，可取四个值：

| `type`   | 作用                                                          |
| -------- | ------------------------------------------------------------- |
| `auto`   | 默认值：使用 `ALL_PROXY` 的设置，未设置时直连                 |
| `direct` | 不使用代理，忽略 `ALL_PROXY`                                  |
| `mtproto`| MTProto 代理：`server`、`port` 和 `secret`                    |
| `socks5` | SOCKS5 代理：`server`、`port`，以及在代理要求时提供 `username`/`password` |

MTProto 代理就是官方客户端从 `tg://proxy` 或 `t.me/proxy` 链接中获取的那种代理，也包括在 `127.0.0.1` 上提供这种代理的本地桥接程序。按代理发布时的形式填写 `secret`：hex 和 base64url 都能识别，三种 secret 都可用：普通 secret、以 `dd` 开头的 secret，以及以 `ee` 开头、把连接伪装成访问 secret 中所含域名的普通网页流量的 secret。

```yaml
proxy:
  type: mtproto
  server: 127.0.0.1
  port: 1443
  secret: ee0123456789abcdef0123456789abcdef6578616d706c652e636f6d
```

请完整复制 secret，不要拆开：在 hex 形式中，`ee` 前缀和伪装域名属于同一个字符串（上例末尾就是 `example.com`），在 base64url 形式中，整个 secret 是一个词。

要求验证身份的 SOCKS5 代理需要同时提供 `username` 和 `password`；只提供其中一项会被拒绝，因为只有一半登录信息的连接会被拒之门外，而且不会给出任何理由。

```yaml
proxy:
  type: socks5
  server: 10.0.0.1
  port: 1080
  username: me
  password: hunter2
```

属于其他类型的值不会被使用，启动时会提示：`socks5` 下的 `secret` 不会生效，`tele` 会明确说出来，而不会让你以为它生效了。

<a id="when-the-proxy-does-not-work"></a>
### 代理不工作时

如果 `tele` 无法读取代理设置，它会停止启动，并给出一条消息，指出是哪个键、在哪个文件中，以及改哪一处就能不经代理连接：

```text
config: proxy.secret is neither hex nor base64url; it is what a proxy publishes,
copied whole (in ~/.config/tele/config.yml; set proxy.type: direct to connect
without a proxy)
```

如果代理无人应答，在一次短超时的连接尝试后，启动同样会停止。这两种情况都发生在界面绘制之前，这是有意为之：这个文件中的其他任何设置项出错时都会恢复为默认值并以警告提示，但连接方式的默认值是直连 Telegram，而 secret 中的一个笔误不应悄悄地把你的流量发往代理本来要避开的地方。

在 Telegram 应答之前，错误的 secret 或错误的登录信息无法与不可达的服务器区分开，所以它们会在 `tele` 运行后表现为普通的连接失败。

<a id="all_proxy"></a>
### ALL_PROXY

在这一节出现之前，`tele` 读取环境变量 `ALL_PROXY`，当 `proxy.type` 为 `auto` 时它仍然会读取。只会读取其中的 `socks5://` 地址；其中的 `http://` 代理会被忽略，其他程序会静默忽略，`tele` 则会给出警告。

这个变量已被弃用，将来会移除。它作用于从该 shell 启动的所有程序，而不仅仅是 `tele`；它无法描述 MTProto 代理；它也不会显示在设置面板中。请把它移到配置中：

```yaml
proxy:
  type: socks5
  server: 127.0.0.1
  port: 1080
```

<a id="state-directory"></a>
## 状态目录

`state_dir` 设置账号状态的存放位置：Telegram 会话、本地数据库和实例锁。默认是 `$XDG_STATE_HOME/tele`，未设置该变量时为 `~/.local/state/tele`。

同一时间只有一个 `tele` 持有某个状态目录：它维护 Telegram 连接、会话和数据库。在同一状态目录上启动的第二个 `tele` 不会再次打开它们，而是作为同一账号的另一个窗口连接到第一个，共用它的连接。关闭第二个窗口，第一个照常运行。关闭第一个，所有连接到它的窗口都会关闭，所以只要还有其他窗口打开，它在退出前就会询问，无论 `confirm_quit` 如何设置。失去第一个 `tele` 的窗口会提示这一点，并在你退出时打印输入框中已输入的内容。

连接到其他 `tele` 的窗口把日志写入 `tele.log` 旁边的 `tele-client.log`，每行都带有进程 id。

太旧而无法被连接的 `tele`（1.11 或较早的 2.0 测试版）仍然会持有状态目录，但不允许任何东西连接它。在它旁边启动的较新版本会等待几秒，然后说明是哪个进程持有该目录，并退出。两者使用不同版本的协议时也是如此。请退出正在运行的那个，然后重新启动。

<a id="running-without-a-window"></a>
## 无窗口运行

`tele daemon` 在没有自己窗口的情况下持有状态目录：它保持连接、发送桌面通知，并允许窗口连接它，直到你用 Ctrl+C 停止它。它只会持有目录：如果已有其他 `tele` 持有，它会给出那个 `tele` 的版本和进程 id，然后退出。

`tele daemon status` 询问持有状态目录的 `tele` 的运行状况：它的版本与你的版本的对比、是什么让它保持运行、运行了多久、连接了多少个窗口、账号处于什么状态、与 Telegram 的连接是否正常、最近一次更新何时到达，以及有多少消息等待发送。有 `tele` 回应时退出码为 0，没有任何回应时为 3。

`tele daemon --service` 由服务管理器运行，不需要手动输入。以这种方式启动的 `tele` 在状态目录被占用时不会退出：它会等待，并在持有目录的 `tele` 退出后接管。

<a id="as-a-service"></a>
### 作为服务运行

`tele daemon install` 让操作系统保持 `tele` 运行：每次登录系统时启动，退出后再次启动。

- macOS：`~/Library/LaunchAgents` 中的 LaunchAgent。它自身的输出写入 `tele.log` 旁边的 `service.log`。
- Linux：`~/.config/systemd/user` 中的 systemd 用户单元；输出进入 journal。不会启用 linger：服务在你登录期间运行，而桌面通知本来也只有在这段时间才能送达。
- Windows：在登录时启动的任务计划程序任务。Windows 服务运行的环境无法显示通知。
- 其他系统上没有可用的安装方式；`install` 会说明如何在你自己的会话中启动 `tele daemon`。

服务按你的 `PATH` 找到的方式运行 `tele`，例如 `/opt/homebrew/bin/tele`，而不是它背后某个具体版本的文件，这样升级会替换它启动的程序。如果你 `PATH` 中的 `tele` 不是你正在运行的那个，`install` 会拒绝并给出两者；`--path` 可以指定改为运行的命令。使用 Scoop 时，`PATH` 中的命令是一个 shim，所以请用 `--path` 指定。

每个配置文件对应一个服务。服务记住安装时使用的配置，并以它命名，所以测试版和稳定版 `tele`，或者两个账号，各有各的服务。重新安装会替换服务并重新启动它，这也是让服务指向一个换了位置的 `tele` 的方法。

如果安装时有 `tele` 持有状态目录，服务会等待，并在那个 `tele` 退出后接管。

你第一次登录的、持有账号的 `tele` 会询问一次是否安装服务：`y` 立即安装，服务会在该 `tele` 退出后接管；`n` 或 Esc 则保持原样。无论哪种回答都会被记住，不会再问。只有在 `install` 能直接成功的情况下才会询问：没有服务管理器时、`PATH` 找到的是另一个 `tele` 时、服务已经安装时都不会询问。连接到其他 `tele` 的窗口从不询问。

升级后，服务会继续运行旧版本，直到有一个较新的窗口连接到它：窗口会请求它重启，服务管理器随后启动当前安装的版本。在此之前，`tele daemon status` 会提示两者版本不同。如果服务运行的命令已不存在，或者版本与你的不同，它也会发出警告。

`tele daemon uninstall` 停止并删除服务。连接到它的窗口会收到服务已被卸载的通知。状态和配置保持原样。

<a id="windows-that-wait"></a>
### 等待中的窗口

连接到服务的窗口在服务消失时会等待它。大约两秒后，状态栏显示正在重新连接；输入框仍然可以编辑，按 Enter 会保留已输入的内容而不是发送。服务恢复后，窗口从原处继续。三十秒后它停止等待，并提示使用 `tele daemon status`；在 Windows 上它会等待九十秒，因为任务计划程序要过一分钟才会重新启动服务。连接到其他任何 `tele` 的窗口不会等待：那个 `tele` 是某人的会话，没有谁会重新启动它。

当较新的 `tele` 连接到较旧的服务时，它会请求服务重启，服务管理器随后以当前安装的版本再次启动它。比服务旧的窗口会收到 `tele` 已更新的通知并关闭；请重新启动它。不是服务的 `tele` 永远不会被外部重启。

如果在没有任何窗口打开时账号被退出登录或被封禁，系统会通过桌面通知告知一次，无论 `ui.notifications.desktop` 如何设置：这是得知消息已不再到达的唯一途径。

旧的 `telegram.session_file` 键仍然有效，会让会话保存在它指向的位置，但它已被弃用，将在下一个版本中移除。如果你没有设置过它，现有的会话和数据库会在首次运行时自动移入状态目录：不会丢失任何内容，你也会保持登录。

<a id="telegrams-limits"></a>
## Telegram 的频率限制

Telegram 限制客户端调用它的频率，对调用过于频繁的客户端会要求等待（flood wait），等待结束前不再接受同类调用。`tele` 会提前把调用控制在限制之内，并在收到等待要求时遵守：

- 所有连接到同一个 `tele` 的窗口和命令对 Telegram 的调用，共用一个额度：一次最多 40 个，之后每秒 20 个。
- 等待只会阻塞触发它的那一类调用：加载历史消息时遇到的等待不会阻塞发送。
- 没有人在等的工作，例如提前下载的原尺寸图片（`photos.eager_full_quality`）和连接时读取的论坛话题，会给你正在做的事留出三分之一的额度，并且会默默等完任何等待。
- 当你正在等的操作遇到两秒或更长的等待时，状态栏会显示 "Telegram asks to wait" 并倒计时。不超过 45 秒的等待会直接等完；更长的等待会被报告，正在发送的消息会留在队列中，等待结束后发出。

Telegram 不公开它的限制，所以上述额度只是估计。如果 `tele` 经常让你等待，或者在你没有做任何特别的事时 Telegram 也要求它等待，请[提交 issue](https://github.com/sorokin-vladimir/tele/issues) 并附上 `tele.log`。

<a id="notifications"></a>
## 通知

新消息通过两种方式通知你，`ui.notifications` 可以分别开关它们。`desktop` 把通知交给操作系统的通知服务，这样即使 `tele` 不在屏幕上也能看到，并且可以由你自己的通知守护进程处理。`toast` 在 `tele` 自己窗口的一角显示弹出提示，任何守护进程规则都管不到它。你可以关闭其中任意一个，或者两个都关。

这两个开关都不影响聊天列表：即使两者都关闭，新消息仍会让对应的行高亮并把聊天移到前面。这是消息到达本身，而不是打扰，与 Telegram 自己的静音划出的界线相同。

`preview` 决定通知的内容，而不是通知的去向，所以对两者都适用：关闭后，桌面通知和弹出提示都只显示发送者的名字。通知正文只生成一次，原样交给两者，因此两者对同一条消息的描述永远一致。

`ui.toasts` 决定弹出提示本身的位置。错误、警告和确认显示在 `error_zone`，通知显示在 `notify_zone`。两者都可取 `bottom-right` 或 `top-right`，也可以指定同一个角，这时它们会叠在一起。不提供左下角：那里留给按键显示。

<a id="photos"></a>
## 图片

`photos.mode` 选择绘制方式。`auto` 在已知能通过 Unicode 占位符放置图片的终端（kitty、Ghostty 和 3.7.0 起的 iTerm2）中使用 Kitty 图形协议，其他地方则绘制 ANSI 字符块画，包括不转发该协议的 tmux 和 screen。`kitty` 和 `blocks` 为启发式规则不认识的终端强制指定一种方式：在忽略占位符的终端上强制使用 `kitty`，图片位置会是一片空白，而不是字符块画。该值只在启动时读取一次。

`kitty_placement_cap` 限制终端上同时存在的 Kitty 图片数量。只传输屏幕上的图片（加上几张刚滚过去的）；更早的会被清除。一次传输整个图片很多的聊天，可能超出终端的图片上限并导致图片错乱（缩小或错位）。如果仍然看到这种情况，请调低这个上限。

`max_long_side_px` 以像素为单位限制聊天中图片的长边（与桌面客户端固定的媒体尺寸相对应）。高度还会额外限制在聊天面板的 2/3 以内，这样高图片不会占满整个视图。想让图片更大就调高，想更紧凑就调低。

<a id="avatars"></a>
## 头像

`avatars.disk_cache_size` 是第二个额度，有意与 `photos.disk_cache_size` 分开。一个头像大约一百 KB，每次打开那个人的资料都会再次用到；而聊天中的媒体数量没有上限，通常只看一次，所以如果共用一个额度，一次长时间的滚动就会把你所有的头像都挤出去。任何一个键设为 `0` 都表示“运行之间不保留任何内容”：对应的缓存会移到临时目录，并在退出时删除。

<a id="see-also"></a>
## 另请参阅

- [主题](themes.md)：`ui.theme` 槽位、编写自己的主题、所有 token
- [快捷键](keybindings.md#configurable-actions)：`keybindings:` 一节
- [媒体](media.md)：图片、语音和视频设置影响什么
