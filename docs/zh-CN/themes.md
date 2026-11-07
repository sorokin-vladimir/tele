# 主题

> 本文译自 [themes.md](../themes.md)，对应提交 `a8ea9ea`。译文与英文版不一致时，以英文版为准。本译文尚未经过母语者审校。如果发现翻译错误，欢迎[提交 issue](https://github.com/sorokin-vladimir/tele/issues)。

tele 绘制的每种颜色都对应一个有名字的 token。**主题**为这些 token 赋值，你也可以编写自己的主题。

<a id="slots"></a>
## 槽位

tele 同时持有两个主题，每个**槽位**一个：终端背景为深色时使用深色槽位，为浅色时使用浅色槽位。tele 跟随终端，随着操作系统或终端的切换而切换。

槽位在 `ui.theme` 下配置，它接受一个名称或一个映射：

```yaml
ui:
  # Nothing set: the built-in tele-dark and tele-light.

  theme: gruvbox-dark        # this theme in both slots, whatever the background

  theme:
    dark: gruvbox-dark       # dark slot from a file
    light: solarized-light   # light slot from another

  theme:
    dark: gruvbox-dark       # dark slot from a file, light slot stays built-in
```

只写一个主题名，两个槽位就都用它，这就是“始终使用这个主题”的写法。没有单独的开关来关闭跟随：两个槽位放的是同一个主题。

内置主题叫 **`tele-dark`** 和 **`tele-light`**。它们是每条继承链的根：没有任何主题设置的 token 会取自该槽位的内置主题，所以主题永远不必完整，tele 新增 token 时也不会因此失效。

> 在主题功能出现之前，tele 写入的每个配置中都有 `ui.theme: default`。它指的是一对主题，而“一对”的概念已经不存在了。它会被忽略并给出警告；请删掉这一行。

<a id="writing-a-theme"></a>
## 编写主题

主题文件放在配置旁边的 `~/.config/tele/themes/` 中，每个文件一个主题，主题以文件命名：`themes/mine.yml` 就是主题 `mine`。`.yml` 和 `.yaml` 都能读取。名称中 `-` 和 `_` 等同，不区分大小写，所以 `my-theme`、`my_theme` 和 `MyTheme` 指的是同一个主题。

最快的入门方式是把当前正在用的主题导出，然后编辑：

```console
tele theme dump > ~/.config/tele/themes/mine.yml
tele theme dump light > ~/.config/tele/themes/mine-light.yml
```

也可以从 tele 已经附带的调色板开始。Catppuccin Macchiato、Dracula、Gruvbox Dark、Nord、Tokyo Night（Night、Moon 和 Day）和 Seoul256 Light 是**随附**主题：它们就在二进制文件里，所以即使机器上完全没有主题文件，`ui.theme: nord` 也能用，不需要复制任何东西。每个随附主题都设置了所有 token 并设定了画布，并且都有注释，说明它来自哪个调色板以及哪些地方不得不偏离原调色板。如果想在选择前先读一读，文件在 [`themes/`](../../themes/) 中。

如果想修改某个随附主题而不只是使用它，可以像下面那样把它指定为基础主题，或者导出后编辑结果：`tele theme dump nord > ~/.config/tele/themes/mine.yml`。

只想改几种颜色时，用继承就够了。`base:` 指定一个主题，本主题没有设置的一切都从它获取：

```yaml
# ~/.config/tele/themes/mine.yml
base: tele-dark
border_pane_active: "#8ec07c"
accent: bright-blue
```

`base:` 可以指定内置主题、随附主题或你的另一个文件，继承链会一直追溯到底。出现循环时会报告，该槽位保留内置主题。

<a id="when-two-themes-have-one-name"></a>
### 两个主题同名时

你的文件优先于随附调色板。在你的机器上，`nord` 指的就是 `~/.config/tele/themes/nord.yml`，它会**完全替换**随附的 `nord`：它没有设置的部分取自内置主题，而不是取自被它占用名字的那个调色板。如果你本意是调整随附主题，请给自己的文件换个名字，并写上 `base: nord`。

两个内置主题是例外。名为 `tele-dark.yml` 或 `tele-light.yml` 的文件会被忽略并给出警告，因为 `base: tele-dark` 在每台机器上都必须指同一个调色板；否则你从别人那里复制来的主题在你这里会解析出和他那里不同的颜色。

<a id="colors"></a>
## 颜色

token 只保存颜色，不保存别的。字重、倾斜、内边距和边框字符属于绘制该元素的组件，而不属于主题，所以这里没有 `bold` 或 `italic` 可设。这是主题目前的样子，并不承诺它永远如此。

token 可以取以下任意形式：

| 形式          | 示例          | 说明                                     |
| ------------- | ------------- | ---------------------------------------- |
| 长 hex        | `"#8ec07c"`   | 要加引号，否则 YAML 会把 `#` 当作注释    |
| 短 hex        | `"#8ec"`      | 展开为 `#88eecc`                         |
| 调色板索引    | `240`         | 0–255                                    |
| ANSI 名称     | `bright-blue` | 见下面的十六个名称                       |
| `none`        | `none`        | 完全没有颜色                             |

十六个名称是 `black`、`red`、`green`、`yellow`、`blue`、`magenta`、`cyan`、`white`，以及它们各自的 `bright-` 形式。

0–15 范围内的索引或名称**不是** hex 值的简写：它们取自你终端配置的调色板。用它们编写主题，tele 就会跟随你的终端，而不是覆盖它。

`none` 表示不设置该属性。用作前景时，文字使用终端自己的文字颜色；用作背景时不绘制任何东西，这是让终端透明效果透出来的唯一方法。在 `background` 上，正是它让整个屏幕保持终端本身的背景：

```yaml
surface_overlay: none
```

`highlight_accent`、`highlight_error`、`highlight_base_chat` 和 `highlight_base_bubble` 不接受 `none`。这四个 token 用于插值而不是直接绘制，“没有颜色”会悄悄变成黑色。

<a id="the-two-list-tokens"></a>
## 两个列表 token

`sender_palette` 按 id 为每个发送者挑选颜色，可以有任意多个条目：

```yaml
sender_palette: ["#ff5f5f", "#5fd75f", bright-blue, none]
```

`logo_gradient` 是启动画面标志循环变化所用的渐变。它至少需要两个色标，按升序排列，从 0 开始，到 1 结束：

```yaml
logo_gradient:
  - { pos: 0.0, color: "#3c5aa0" }
  - { pos: 0.5, color: "#82aae6" }
  - { pos: 1.0, color: "#d7eeff" }
```

设置其中任何一个都会**替换**从基础主题继承的列表，而不是与之合并。想改一个发送者的颜色，就要把所有颜色都写出来。

<a id="when-something-looks-wrong"></a>
## 显示不对劲时

```console
tele theme check
```

会打印每个槽位最终解析成了什么、它沿着哪条链继承，以及有多少 token 来自何处，包括那些因为链上没有任何主题设置而回退到内置主题的 token。`tele theme check mine` 按名称检查一个主题。

链上的每个主题都会打印出它的来源，而这正是屏幕无法告诉你的：

```
dark slot: nord
  chain: nord (file) <- tele-dark (built-in)
  note: nord is also a bundled theme; the file is used
```

`file` 是你的文件，`bundled` 是随二进制文件附带的调色板，`built-in` 是 `tele-dark` 或 `tele-light`。

tele 无法使用的任何东西（不存在的主题、无法读取的文件、无法解析的颜色、不认识的键）都只是警告，绝不会导致无法启动。警告会在 tele 打开时以弹出提示显示，并写入 `~/.local/state/tele/tele.log`。不认识的键会被忽略，文件的其余部分照常使用，所以为较新版本 tele 编写的主题在这里也能用。

<a id="what-will-not-be-readable"></a>
### 哪些会难以辨认

如果你的主题设置了 `background`，`tele theme check` 还会测量直接绘制在该画布上的一切（每个前景 token 和每个 `sender_palette` 条目），并列出与画布对比度低于 3.0:1 的项：

```
  canvas #ffffff: 15 tokens and 7 sender_palette entries below 3.0:1
    markup_link           1.09:1   from mine
    sender_palette[4]     1.22:1   from tele-dark
    name_editing          1.39:1   from tele-dark
    …
```

最差的排在最前，因为 1.1:1 完全看不见，而 2.8:1 只是比较淡。要看的是最后一列：`from mine` 表示这个颜色是你写的，你可以改；`from tele-dark` 表示它是继承来的，你需要在自己的文件中设置这个 token。上面的例子是常见的意外：浅色画布配上以 `tele-dark` 为根的继承链，而 `tele-dark` 提供的是为黑色背景调校的前景调色板。

保持为 `none` 的 token 也会列出，但没有对比度数值。它无法测量：`none` 表示终端自己的前景色，与你的画布毫无关系。

启动和重新加载时，每个主题只会有一条弹出提示，而不是每个 token 一条，告诉你有多少项以及详情在哪里：

```
theme mine: 15 tokens and 7 sender_palette entries are unreadable on its canvas; run tele theme check
```

这些都不会拒绝任何东西。主题会完全按照你写的样子加载和绘制。这个下限是参考而不是规则，它设在 3.0:1 而不是正文所用的 4.5:1，正是为了让本来就想要淡一些的 token 可以保持淡。如果你不同意某项结果，忽略它即可。

这是有意为之，而不是没做完。对比度根据相对亮度计算，看不到色相，所以在饱和色与中性色之间会误判：tele-dark 自己的强调色与它所标记的文字之间只有 1.29:1，却完全清晰可读。用一种已知会以特定方式出错的度量来强制执行某个阈值，会拒绝本来没问题的主题，而且没有申诉的余地。警告以后可以根据证据收紧；拒绝一旦实施，要放宽就得先弄坏人们已经写好的文件。

有两件事它不检查。没有设置 `background` 的主题完全不做检查：这时画布属于终端，tele 看不到它；内置主题则由它们自己的测试覆盖。另外，绘制在应用自己绘制的表面上的 token（`text_on_toast`、`accent_status_bar`）在这里也不测量：它们应该相对于那个表面来判断，而不是相对于画布。

<a id="editing-a-theme-while-tele-runs"></a>
## 在 tele 运行时编辑主题

`reload_config` 操作会重新读取主题文件和配置文件并应用两者，无需重启。它默认**没有绑定按键**，因为只有在你在另一个窗口里编辑东西时它才有用，最常见的是编写主题时，此时每试一种颜色就重启一次，都要付出重新连接的代价。需要时再绑定它：

```yaml
keybindings:
  global:
    reload_config: ctrl+t
```

`reload_themes` 是这个操作只读取主题文件时的名字。它仍然有效，作用相同，所以已经绑定了它的配置无需修改。

重新加载会报告它的发现：每个槽位解析出的主题，或者第一个问题以及日志中还有多少个其他问题。配置文件中的任何错误（tele 不接受的值、不存在的主题）也以同样方式报告。

<a id="tokens"></a>
## Token

运行 `tele theme dump` 可以看到每个 token 的当前值。每个 token 负责绘制的部分：

<a id="surfaces---filled-areas-drawn-behind-content"></a>
### 表面：绘制在内容背后的填充区域

| Token                  | 位置                                                   |
| ---------------------- | ------------------------------------------------------ |
| `surface_overlay`      | 弹出菜单、反应选择器、提及弹窗                         |
| `surface_help`         | 帮助窗口面板                                           |
| `surface_toast`        | 弹出提示面板                                           |
| `surface_status_bar`   | 状态栏                                                 |
| `surface_selected`     | 选中行的填充、提及弹窗的边框、搜索提示                 |
| `surface_self_mention` | 提及你的 @mention                                      |
| `surface_code`         | 行内代码和代码块                                       |

<a id="text"></a>
### 文字

| Token                    | 位置                                                                              |
| ------------------------ | --------------------------------------------------------------------------------- |
| `text`                   | 正文：消息文字、聊天标题、文件夹名称、搜索结果行、未读计数                        |
| `text_dim`               | 时间戳、引用、分隔线                                                              |
| `text_muted`             | 已静音的聊天                                                                      |
| `text_faint`             | “no results”、“empty”、面板中的提示说明                                           |
| `text_subtle`            | 弹出提示的溢出行                                                                  |
| `text_on_surface`        | 帮助窗口正文                                                                      |
| `text_status_bar`        | 状态栏正文                                                                        |
| `text_on_selected`       | `surface_selected` 上的文字                                                       |
| `text_on_selected_muted` | `surface_selected` 上的次要文字                                                   |
| `text_on_toast`          | 弹出提示正文                                                                      |
| `text_mode_label`        | NORMAL/INSERT 标签                                                                |
| `text_code`              | 行内代码和代码块                                                                  |

`text` 是屏幕上面积最大的颜色区域，决定了一个主题看起来像不像它自己。内置主题把它保持为 `none`，即终端自己的前景色，和这个 token 出现之前 tele 的表现完全一样，所以主题必须有意地设定它。

<a id="the-canvas"></a>
### 画布

`background` 是一切背后的颜色区域，在没有被任何表面覆盖的地方都能看到。两个内置主题都把它保持为 `none`，即终端自己的背景，tele 一直都是这个样子，所以在某个主题设定它之前，什么都不会变。

| Token        | 位置                                                 |
| ------------ | ---------------------------------------------------- |
| `background` | 整个屏幕，在每个面板、栏和气泡的背后                 |

**`background` 需要 `text`。** 只设置其中之一的主题会被拒绝并给出警告，绘制时就像两个都没设置。原因不在于整洁：把屏幕涂上颜色、却把文字交给终端，等于把已知的背景放在未知的前景下面。这种组合曾经在面板层面出现过一次：在浅色终端上弹出菜单变成了灰底蓝字，强调显示的快捷键与周围的标签无法区分。在画布层面，它会覆盖一切。

这项要求检查的是整条 `base:` 链得出的主题，而不是你眼前的这个文件，所以把两者分在不同层里是可以的：

```yaml
# palette.yml
text: "#cdd6f4"

# mine.yml
base: palette
background: "#1e1e2e" # accepted: the chain supplies text
```

这项要求只在一个方向上成立。只有 `text` 没有 `background` 是合法的，如果内置主题要设定正文颜色，也会这样做。

**设置它会结束终端透明效果。** 透明终端会透过 tele 没有绘制的单元格显示你的壁纸；画布会绘制所有单元格。这件事没有折中的做法。想保留背景，就不要设置 `background`。

在设置它之前，还有两件事值得了解：

- 其他所有绘制在画布*上*的 token（`text_dim`、`status_online`、`tick_read`、`name_incoming`、整个 `sender_palette`）仍然来自你的继承链的根。一个绘制浅色画布、却以 `tele-dark` 为根的主题，会继承为黑色背景调校的前景调色板，其中有几个 token 会几乎看不见。设置画布后请运行 `tele theme check`：一旦主题指定了自己的画布，tele 就能根据它测量这些 token，并告诉你是哪些。参见[哪些会难以辨认](#what-will-not-be-readable)。
- 图片照常工作。Kitty 图片绘制在它所占的单元格之上，画布会在图片透明的地方透出来，这其实是一项改进：没有画布时，透明像素显示的是终端，在原本已经涂满的屏幕上留下图片形状的洞。

<a id="accents"></a>
### 强调色

强调色有三个，因为强调色会画在三种不同的背景上，而在浅色主题中这些背景并不一致：终端背景是浅色的，但状态栏以及（在大多数主题中）弹出面板不是。

| Token                | 位置                                                                                           |
| -------------------- | ---------------------------------------------------------------------------------------------- |
| `accent`             | 在终端背景上：图片、视频和搜索的提示，它们背后没有面板                                         |
| `accent_on_surface`  | 在应用自己绘制的面板上：帮助窗口、弹出菜单、选择器中的编号、弹出提示中的操作                   |
| `accent_status_bar`  | 状态栏上，NORMAL 模式                                                                          |
| `accent_insert`      | 状态栏上，INSERT 模式                                                                          |
| `accent_mode_normal` | NORMAL 标签的填充                                                                              |
| `accent_mode_insert` | INSERT 标签的填充                                                                              |

如果你的主题在浅色终端下绘制深色状态栏（`tele-light` 就是这样），请让 `accent_status_bar` 和 `accent_insert` 保持浅色。只有这两个强调色的背景不跟随终端。

<a id="status-and-message-state"></a>
### 状态与消息状态

| Token                                                  | 位置                                   |
| ------------------------------------------------------ | -------------------------------------- |
| `status_error`、`status_warning`、`status_info`        | 弹出提示和状态消息                     |
| `status_online`                                        | 在线圆点                               |
| `tick_sent`、`tick_outbox`、`tick_read`、`tick_failed` | 送达勾号                               |
| `name_incoming`                                        | 收到的消息的发送者名字                 |
| `name_editing`                                         | 编辑时的名字                           |
| `indicator`                                            | 气泡旁的选中条                         |
| `unread_separator`                                     | 未读分隔线                             |
| `waveform_played`                                      | 语音波形中已播放的部分                 |
| `reaction_chosen`                                      | 你做出的反应                           |
| `unread_reaction`                                      | 聊天列表中的未读反应标记               |
| `unread_mention`                                       | 聊天列表中的未读提及标记               |

<a id="borders"></a>
### 边框

| Token                                   | 位置                             |
| --------------------------------------- | -------------------------------- |
| `border_pane_active`                    | 当前聚焦的面板                   |
| `border_bubble_in`、`border_bubble_out` | 收到的和发出的气泡               |
| `border_overlay`                        | 帮助窗口                         |
| `border_composer_focused`               | 获得焦点的输入框                 |
| `border_composer_flash`                 | 达到长度上限时的输入框           |
| `border_status_sep`                     | 状态栏分隔符                     |

<a id="message-markup"></a>
### 消息标记

| Token                    | 位置                                       |
| ------------------------ | ------------------------------------------ |
| `markup_link`            | 网址、邮箱、电话号码、卡号                 |
| `markup_ref`             | 提及、话题标签、股票代码标签、机器人命令   |
| `markup_self_mention_fg` | 提及你的 @mention 的文字                   |

<a id="highlights"></a>
### 高亮

| Token                   | 位置                                     |
| ----------------------- | ---------------------------------------- |
| `highlight_accent`      | 跳转提示，逐渐淡向它的基色               |
| `highlight_error`       | 被回滚的操作                             |
| `highlight_base_chat`   | 聊天行高亮淡向的色调                     |
| `highlight_base_bubble` | 气泡高亮淡向的色调                       |
| `overlay_dim`           | 模态窗口背后的内容                       |

前四个用于插值，所以不接受 `none`。

<a id="composer"></a>
### 输入框

| Token                  | 位置                                      |
| ---------------------- | ----------------------------------------- |
| `composer_counter_dim` | 空闲时的字数计数                          |
| `composer_glyph_idle`  | 空闲时的输入框图标                        |
| `composer_glyph_ready` | 有内容可发送时的输入框图标                |

<a id="lists"></a>
### 列表

| Token            | 位置                                 |
| ---------------- | ------------------------------------ |
| `sender_palette` | 发送者名字的颜色，按 id 挑选         |
| `logo_gradient`  | 启动画面标志的渐变                   |
