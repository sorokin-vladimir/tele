# tele

> 本文译自 [README.md](README.md)，对应提交 `04bd26c`。译文与英文版不一致时，以英文版为准。本译文尚未经过母语者审校，欢迎提交修正。

```
  _            _
 | |_    ___  | |   ___
 | __|  / _ \ | |  / _ \
 | |_  |  __/ | | |  __/
  \__|  \___| |_|  \___|
```

> 为键盘操作而生的终端原生 Telegram 客户端。

<p align="center"><a href="README.md">🇬🇧 English</a> · <a href="README.ru.md">🇷🇺 Русский</a></p>

[![Go](https://img.shields.io/badge/go-1.26+-blue)](https://go.dev)
[![License](https://img.shields.io/badge/license-GPL--3.0-green)](LICENSE)
[![Release](https://img.shields.io/github/v/release/sorokin-vladimir/tele)](https://github.com/sorokin-vladimir/tele/releases)
[![Downloads](https://img.shields.io/github/downloads/sorokin-vladimir/tele/total?color=blue)](https://github.com/sorokin-vladimir/tele/releases)
[![Stars](https://img.shields.io/github/stars/sorokin-vladimir/tele?style=flat)](https://github.com/sorokin-vladimir/tele/stargazers)
[![Last commit](https://img.shields.io/github/last-commit/sorokin-vladimir/tele)](https://github.com/sorokin-vladimir/tele/commits/main)
[![Platform](https://img.shields.io/badge/platform-macOS%20%C2%B7%20Linux%20%C2%B7%20Windows-lightgrey)](#installation)

<p align="center">
  <a href="#features">功能</a> •
  <a href="#installation">安装</a> •
  <a href="#why-tele">为什么选 tele？</a> •
  <a href="#compared-to-other-terminal-clients">与其他 TUI 客户端对比</a> •
  <a href="#keybindings">快捷键</a> •
  <a href="#roadmap">路线图</a> •
  <a href="#documentation">文档</a>
</p>

---

![tele demo](./assets/demo.gif)

> **状态：** 一个完整的终端 Telegram 客户端，作者每天都在使用：聊天、群组、频道和论坛、媒体、反应、文件夹，以及在没有打开窗口时也能收到的通知。尚未支持的功能列在[路线图](#roadmap)中。

---

<a id="why-tele"></a>
## 为什么选 `tele`？

Telegram Desktop、网页版和手机应用都是围绕鼠标操作设计的。

如果你的日常工作都在终端里，用着 Neovim、yazi、k9s 或 tmux 这样的工具，切换到图形界面的聊天软件会打断你的节奏。

`tele` 让你留在终端里。

它为以下场景而做：

- 用键盘导航
- 快速切换聊天
- 通过 SSH 远程工作
- 不受干扰地聊天

如果你觉得 lazygit 这类工具用着顺手，`tele` 也会如此。

它还很轻量：空闲时通常只占约 50MB RSS，而桌面客户端要占几百 MB。

---

<a id="compared-to-other-terminal-clients"></a>
## 与其他终端客户端对比

几乎所有终端 Telegram 客户端都构建在 Telegram 的 C++ 库 TDLib 之上，要么直接链接，要么通过 Python 运行时调用。`tele` 通过 [gotd/td](https://github.com/gotd/td) 自己实现 MTProto，并且不依赖 cgo 构建，所以不需要下载或编译 C++ 库，不需要在机器上保留解释器，也不需要在应用完全可用之前安装一连串可选的辅助程序。

|                  | `tele`                                                           | [tgt](https://github.com/FedericoBruzzone/tgt)                                                                  | [tg](https://github.com/paul-nameless/tg) · [tuigram](https://codeberg.org/Yehoslav/tuigram)                              |
| ---------------- | ---------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------- |
| 语言             | Go                                                               | Rust                                                                                                            | Python                                                                                                                    |
| Telegram 后端    | gotd/td，纯 Go 实现的 MTProto                                    | TDLib                                                                                                           | 通过 `python-telegram` 使用 TDLib                                                                                         |
| 需要安装什么     | 一个静态二进制文件                                               | `cargo install`，加上下载或自行构建的 TDLib；语音消息需要 CMake；聊天内图片需要系统安装的 `chafa`               | Python 3.9+/3.10+ 运行时加 TDLib；完整功能还需要 `ffmpeg`、`terminal-notifier`、`urlview`、`ranger`/`fzf`                 |
| 软件包           | brew、apt、dnf、zypper、apk、nix、scoop、winget、签名的 deb/rpm  | crates.io、AUR、nix、Docker                                                                                     | PyPI、AUR、Docker                                                                                                         |
| 聊天内图片       | Kitty 图形协议原画质显示，回退为 ANSI 字符块画                   | `chafa` 字符块画，需开启可选的构建特性                                                                          | 通过 mailcap 交给外部查看器                                                                                               |
| Windows          | 二进制文件、Scoop、winget                                        | 支持                                                                                                            | 实际不可用                                                                                                                |
| 无窗口时         | 从登录起作为服务运行；没有窗口也能收到通知                       | 只在它的终端打开时运行；未声明支持桌面通知                                                                      | 只在它的终端打开时运行；通过外部命令通知（`terminal-notifier`、`dunst`）                                                  |
| 发布节奏         | 每周发布稳定版，另有独立的测试版渠道                             | 最新版本为 `v1.0.0-rc1`；最近的提交都是依赖更新                                                                 | `tg` 隔几个月集中发布一次；`tuigram` 是一个分叉的分叉                                                                     |

除了打包方式，`tele` 还支持现代 Telegram 中这三者都没有声明支持的部分：反应、聊天文件夹、以相册形式发送，以及视频、圆形视频和 GIF 的聊天内预览。语音消息在应用内播放，带有波形和移动的播放位置，不需要外部播放器，也不需要用 CMake 构建 Opus 解码器。聊天从本地 SQLite 历史中即时打开；主题以八个内置调色板的形式提供，会随着终端背景在深色和浅色之间切换（`tele theme check` 可以检查对比度）；每个按键都能按上下文重新绑定，包括组合键序列。

它们领先的地方：`tg` 和 `tuigram` 可以录制语音消息（通过 `ffmpeg`），并支持私密聊天。前者不在 `tele` 的范围内（[原因](docs/zh-CN/media.md#recording-is-out-of-scope)），后者在 [待办列表](https://github.com/sorokin-vladimir/tele/issues/234)中。全文消息搜索也还在[路线图](#roadmap)上。

已完全停止维护的项目没有列入表格：`TelegramTUI`（已标记为弃用，最后一次修改在 2022 年）、`arigram`（已归档）、`tg-tui`（2018 年）、`Telegram-TUI`、`tg9` 和 `ithil`（早期原型）。

---

<a id="features"></a>
## 功能

- **键盘优先**：vim 风格的导航（`gg/G`、插入模式），还有逐条消息移动的光标（`j/k`），它在气泡间逐个跳转，是上下文菜单和消息操作的作用对象。鼠标支持是可选的：可以点击聊天或面板，用滚轮滚动。
- **没有窗口也能收到通知**：`tele daemon install` 让 `tele` 从登录系统起保持运行（launchd、systemd 用户单元或任务计划程序），所以关闭窗口后账号仍然在线，桌面通知照常到达。再次打开 `tele` 时，它会连接到正在运行的那个，而不是再建立一次连接；在两个终端中打开，两边显示的是同一个账号：[docs/zh-CN/configuration.md](docs/zh-CN/configuration.md#running-without-a-window)
- **完整的 Telegram 支持**：私聊、群组、频道、论坛话题、回复、反应、编辑、转发，以及与 Telegram 同步的每个聊天的草稿。
- **聊天内图片**：在 kitty、Ghostty 和 iTerm2 3.7.0+ 中通过 Kitty 图形协议以原画质显示，其他终端用 ANSI 字符块画显示。`o` 在应用内查看器中打开图片。
- **语音消息**：带时长的波形，按 `p` 在应用内播放，并有移动的播放位置。无需外部播放器，也不依赖 cgo。
- **视频、圆形视频和 GIF**：聊天内的缩略图，选中的 GIF 会在原位循环播放。
- **发送媒体**：用 `u` 附加文件；再按一次可以添加更多文件，作为一个相册发送。
- **和官方客户端一样的代理**：MTProto 代理（服务器、端口以及 `tg://proxy` 链接中的 secret，包括 fake-TLS）或 SOCKS5 代理，只在配置中为 `tele` 设置，而不是为整个 shell 设置，并且每个连接都会使用它，包括下载：[docs/zh-CN/configuration.md](docs/zh-CN/configuration.md#proxy)
- **终端原生设计**：从一开始就为终端工作流编写，而不是从图形客户端移植过来。单个静态 Go 二进制文件，启动快，内存占用低。
- **简单的配置**：一个带合理默认值的 YAML 文件，以及按 `,` 打开的设置面板，它编辑的就是同一个文件，注释等内容都会保留。

> **不支持录制。** `tele` 可以*发送*预先录好的音频或视频文件，但不能录制语音消息或圆形视频，也不支持语音或视频通话：作为不依赖 cgo 的终端原生客户端，它没有采集麦克风或摄像头的能力。这是架构上的边界，路线图中也没有相关计划。

各种媒体类型、应用内播放以及可选依赖 `ffmpeg` 的详细说明：[docs/zh-CN/media.md](docs/zh-CN/media.md)

---

<a id="installation"></a>
## 安装

<a id="any-unix-macos--linux--freebsd--openbsd--netbsd---one-liner"></a>
### 任意 Unix（macOS · Linux · FreeBSD · OpenBSD · NetBSD）：一行命令

```sh
curl -sL https://raw.githubusercontent.com/sorokin-vladimir/tele/main/scripts/install.sh | sh
```

脚本会检测你的操作系统和架构，并从最新版本中安装对应的二进制文件。可选参数：

```sh
# 最新测试版（预发布），安装为 `tele-beta`，可与稳定版共存：
curl -sL https://raw.githubusercontent.com/sorokin-vladimir/tele/main/scripts/install.sh | sh -s -- --beta

# 指定版本，或自定义安装目录：
curl -sL .../install.sh | sh -s -- --version v1.9.0
curl -sL .../install.sh | PREFIX="$HOME/.local/bin" sh
```

在 FreeBSD 上，桌面通知通过终端发送（仅限支持此功能的终端），音频播放需要正在运行的 PulseAudio/PipeWire 服务器。缺少它们时，相应功能会自动关闭而不报错。

<a id="macos--linux---homebrew"></a>
### macOS / Linux：Homebrew

```sh
brew install tele  # 来自 homebrew/core
```

或者使用 `tele` 自己的 tap：

```sh
brew tap sorokin-vladimir/tap
brew trust sorokin-vladimir/tap
brew install sorokin-vladimir/tap/tele
```

两个 formula 都特意写了全名。添加 tap 之后，直接运行 `brew install tele` 会选用 tap 中的 formula，而且只用一行 `Warning: tele shadows homebrew/core/tele` 提示，很容易在滚动时错过。两者有一处值得了解的区别，见[应用密钥](#app-key)。

<a id="macos--linux---homebrew-beta-channel"></a>
### macOS / Linux：Homebrew（测试版渠道）

想比每周的稳定版更早用上最新合并的改动，可以从同一个 tap 安装测试版。它是一个独立的 `tele-beta` 二进制文件，有自己的配置和状态（`~/.config/tele-beta`），所以可以和稳定版并存：

```sh
brew tap sorokin-vladimir/tap
brew trust sorokin-vladimir/tap
brew install tele-beta
brew upgrade tele-beta   # 有新测试版时拉取更新
```

测试版来自预发布标签（`vX.Y.Z-beta.N`），在 GitHub 上以预发布版本发布，所以永远不会显示为“最新”版本。用 `tele-beta` 运行。

<a id="linux---binary"></a>
### Linux：二进制文件

```sh
curl -sL https://github.com/sorokin-vladimir/tele/releases/latest/download/tele-linux-amd64 \
  -o ~/.local/bin/tele && chmod +x ~/.local/bin/tele
```

arm64 上把 `amd64` 换成 `arm64`。

<a id="debian--ubuntu--mint---apt"></a>
### Debian / Ubuntu / Mint：apt

```sh
echo 'deb [trusted=yes] https://apt.fury.io/sorokin-vladimir/ /' \
  | sudo tee /etc/apt/sources.list.d/tele.list
sudo apt update && sudo apt install tele
```

<a id="fedora--rhel--opensuse---dnf--zypper"></a>
### Fedora / RHEL / openSUSE：dnf / zypper

```sh
sudo tee /etc/yum.repos.d/tele.repo <<'EOF'
[tele]
name=tele
baseurl=https://yum.fury.io/sorokin-vladimir/
enabled=1
gpgcheck=0
EOF
sudo dnf install tele   # 或：sudo zypper install tele
```

<a id="alpine---apk"></a>
### Alpine：apk

```sh
echo 'https://alpine.fury.io/sorokin-vladimir/' | sudo tee -a /etc/apk/repositories
sudo apk add --allow-untrusted tele
```

> 每个[版本](https://github.com/sorokin-vladimir/tele/releases/latest)也附带了签名的 `.deb` 和 `.rpm` 文件。

> AUR 和 Snap 软件包已在计划中，但尚未发布。

<a id="windows---binary"></a>
### Windows：二进制文件

用 PowerShell 下载可执行文件（以普通用户身份运行）：

```powershell
$dir = "$Env:LOCALAPPDATA\Programs\tele"
New-Item -ItemType Directory -Force -Path $dir | Out-Null
Invoke-WebRequest `
  -Uri "https://github.com/sorokin-vladimir/tele/releases/latest/download/tele-windows-amd64.exe" `
  -OutFile "$dir\tele.exe"
```

arm64 上把 `amd64` 换成 `arm64`。把 `$dir` 加入 `PATH`（或者用完整路径运行 `tele.exe`），然后在任意终端中启动。在 Windows 上图片显示为 ANSI 字符块画：聊天内图片需要终端能通过 Unicode 占位符单元格绘制 Kitty 图片，而目前还没有 Windows 终端能做到。[WezTerm](https://wezterm.org) 支持 Kitty 图形协议，但不支持占位符（[wezterm#7924](https://github.com/wezterm/wezterm/pull/7924)）。一旦有终端支持了它们，在自动检测学会识别该终端之前，`photos.mode: kitty` 就能开启聊天内图片。

> 每个[版本](https://github.com/sorokin-vladimir/tele/releases/latest)都附带一个包含 `tele.exe` 的 `.zip`（`tele_windows_amd64.zip`）。

> 通过包管理器安装：[Scoop](https://scoop.sh) 或 [winget](https://learn.microsoft.com/windows/package-manager/)：
>
> ```powershell
> scoop bucket add tele https://github.com/sorokin-vladimir/scoop-tele
> scoop install tele
> # 或
> winget install sorokin-vladimir.tele
> ```

<a id="nix--nixos"></a>
### Nix / NixOS

`tele` 提供了一个 flake。无需安装即可试用：

```sh
nix run github:sorokin-vladimir/tele
```

或者安装到你的 profile 中：

```sh
nix profile install github:sorokin-vladimir/tele
```

作为你自己的 flake 的输入引入（例如 NixOS 配置）：

```nix
{
  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    tele = {
      url = "github:sorokin-vladimir/tele";
      inputs.nixpkgs.follows = "nixpkgs";
    };
  };
}
```

然后在你的配置中：

```nix
environment.systemPackages = [
  inputs.tele.packages.${pkgs.system}.default
];
```

仓库中还提供了用于本地开发的 `nix develop` 环境（go、golangci-lint、lefthook）。

---

<a id="first-launch"></a>
## 首次启动

```sh
tele
```

首次运行时，`tele` 会创建：

```text
~/.config/tele/config.yml
```

然后依次询问：

- 手机号
- 短信验证码
- 两步验证密码（如果设置了）

登录后，`tele` 会询问一次是否作为服务在后台持续运行。回答 `y`，关闭窗口后账号仍然在线，通知照常到达；回答 `n` 或按 Esc，`tele` 只在窗口打开时运行，和以前一样。之后可以用 `tele daemon install` 或 `tele daemon uninstall` 改变主意。在无法安装服务的地方，或者服务已经安装时，不会询问。

<a id="upgrading-from-111"></a>
### 从 1.11 升级

- 在已有 `tele` 运行时再打开一个，不会再报错：第二个会作为同一账号的另一个窗口连接到第一个。
- 登录后，`tele` 会询问一次是否作为服务运行（见上文）。
- 参数现在使用两个短横线（`--config`、`--debug`）；短参数 `-e` 仍然可用。版本号和主题工具改成了命令：`tele version`、`tele theme check`、`tele theme dump`。
- 1.11 和 2.0 不能同时使用同一个账号：启动 2.0 前请先退出 1.11。

完整的变更记录（英文）：[CHANGELOG.md](CHANGELOG.md)

<a id="app-key"></a>
### 应用密钥

`tele` 通过应用密钥连接 Telegram。应用密钥是一对 `api_id` 和 `api_hash`，它标识的是应用程序，而不是你本人。你不需要为它做任何事：每个构建都自带一个，上面的登录可以直接进行。

从公开源码编译的构建（homebrew-core、Nix flake、BSD 移植版、你自己的 `go build`）共用同一个密钥。Telegram 可能会拒绝它认为被过多人共用的密钥，这时 `tele` 会提示 `app key blocked by Telegram`，而不会误报你已退出登录。有两种解决办法，选更省事的那种即可。

在 [my.telegram.org](https://my.telegram.org) 注册自己的密钥，然后写入 `~/.config/tele/config.yml`。你提供的密钥总是优先于内置密钥：

```yaml
telegram:
  api_id: 123456
  api_hash: "your api hash"
```

两项要么都填，要么都不填：如果配置中只写了一项，`tele` 在启动时就会停止。

或者安装自带独立密钥的官方构建：上面的 Homebrew tap、安装脚本、apt、dnf、apk、Snap、Scoop 和 winget 提供的都是由 `tele` 发布流水线构建的二进制文件。

更多内容，包括哪个 Homebrew formula 带哪个密钥：[docs/zh-CN/app-key.md](docs/zh-CN/app-key.md)

---

<a id="commands"></a>
## 命令

| 命令                      | 说明                                                                                            |
| ------------------------- | ----------------------------------------------------------------------------------------------- |
| `tele`                    | 打开一个显示账号的窗口                                                                          |
| `tele version`            | 打印版本号                                                                                      |
| `tele theme check [NAME]` | 说明每个槽位解析成了哪个主题以及 token 来自何处，或者对按名称指定的一个主题做同样检查           |
| `tele theme dump [NAME]`  | 把槽位的主题或指定名称的主题输出为完整的主题文件（`tele theme dump light`）                     |
| `tele daemon`             | 在没有窗口的情况下持有账号，保持连接和通知；窗口可以连接到它                                    |
| `tele daemon status`      | 说明持有账号的 `tele` 的运行状况（`--json` 供脚本使用）；没有时以退出码 3 结束                  |
| `tele daemon install`     | 让 `tele` 作为服务在每次登录时运行：launchd、systemd 用户单元或任务计划程序                     |
| `tele daemon uninstall`   | 停止并删除服务；状态和配置保持不变                                                              |

| 参数                    | 说明                                                                                           |
| ----------------------- | ---------------------------------------------------------------------------------------------- |
| `-c`, `--config <path>` | 配置文件路径（默认 `~/.config/tele/config.yml`）                                               |
| `-e`, `--debug`         | 启用调试日志                                                                                   |
| `--trace`               | 记录敏感元数据（对方 id、消息长度）。切勿在共享或同步的文件系统上使用                          |

所有命令和参数的完整说明：[docs/zh-CN/commands.md](docs/zh-CN/commands.md)。退出码、JSON 输出和命令的命名规则：[docs/zh-CN/cli.md](docs/zh-CN/cli.md)。无窗口运行：[docs/zh-CN/configuration.md](docs/zh-CN/configuration.md#running-without-a-window)。

---

<a id="keybindings"></a>
## 快捷键

| 按键                | 操作                                              |
| ------------------- | ------------------------------------------------- |
| `j` / `k`           | 在聊天间移动，或选择下一条 / 上一条消息           |
| `ctrl+j` / `ctrl+k` | 滚动消息                                          |
| `i`                 | 撰写消息                                          |
| `r`                 | 回复                                              |
| `e` / `d`           | 编辑 / 删除消息                                   |
| `t`                 | 做出反应                                          |
| `f`                 | 转发消息                                          |
| `u`                 | 附加文件（重复按可发送相册）                      |
| `o`                 | 打开 / 查看媒体（图片在查看器中，视频在聊天中）   |
| `O`                 | 在外部播放器中打开视频                            |
| `p`                 | 在应用内播放语音消息                              |
| `s`                 | 下载选中的文件                                    |
| `/`                 | 搜索聊天                                          |
| `space`             | 消息的上下文菜单                                  |
| `0` / `1` / `2`     | 聚焦面板                                          |
| `,`                 | 设置                                              |
| `g L`               | 退出登录                                          |
| `?`                 | 快捷键一览                                        |
| `q`                 | 退出                                              |

完整列表：[docs/zh-CN/keybindings.md](docs/zh-CN/keybindings.md)

---

<a id="configuration"></a>
## 配置

所有配置都在一个 YAML 文件 `~/.config/tele/config.yml` 中，首次运行时创建。

按 `,` 打开设置面板：其中列出 `tele` 的所有设置项，顺序和分组与文件一致，并显示每项的当前值以及修改何时生效。修改会直接写入 `config.yml`，保留你的注释和空行，所以手动编辑文件和在面板中编辑是一回事。

最常修改的设置：

```yaml
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

photos:
  mode: auto # auto | kitty | sixel | blocks - inline image renderer
  disk_cache_size: 268435456 # 256 MB of fetched chat media kept between runs
```

其他所有键及其作用：[docs/zh-CN/configuration.md](docs/zh-CN/configuration.md)

<a id="themes"></a>
### 主题

`tele` 同时持有两个主题，并随终端背景的变化在它们之间切换：`ui.theme.dark` 和 `ui.theme.light`。不设置 `ui.theme` 时，使用内置的 `tele-dark` 和 `tele-light`。

二进制文件中附带了八个现成的主题：Catppuccin Macchiato、Dracula、Gruvbox Dark、Nord、Tokyo Night（Night、Moon 和 Day）和 Seoul256 Light。无需安装或复制任何东西：写上名字就能用。

```yaml
ui:
  theme: tokyonight-night
```

你自己的主题是 `~/.config/tele/themes/` 中的文件，主题只需设置它关心的 token，其余的会继承。随附主题的源文件带有详细注释，说明每个主题移植自哪个调色板，位于 [`themes/`](themes/)。

编写主题、所有 token 以及对比度检查：[docs/zh-CN/themes.md](docs/zh-CN/themes.md)

<a id="customizing-keybindings"></a>
### 自定义快捷键

在配置文件的 `keybindings:` 一节中覆盖默认按键。生成的配置中已经以注释形式列出了每个操作及其当前默认按键，并按上下文分组。

按键语法、组合键序列、冲突的处理方式以及完整的操作名列表：[docs/zh-CN/keybindings.md](docs/zh-CN/keybindings.md#customizing-keybindings)

---

<a id="roadmap"></a>
## 路线图

计划中的工作都在公开的[**项目看板**](https://github.com/users/sorokin-vladimir/projects/2)上，按[里程碑](https://github.com/sorokin-vladimir/tele/milestones)分组。里程碑对应次版本（`v1.11`、`v2.0`……）；补丁版本在修复完成后于该版本线内陆续发布。

| 版本                | 重点                                                                                                                                                                                                                                     |
| ------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `v1.11` _(已完成)_  | 设置与用户：应用内设置面板、用户资料面板、头像、内置于二进制文件的主题，以及遗留的分发和审查问题                                                                                                                                         |
| `v2.0` _(已完成)_   | Telegram 连接的唯一持有者：连接到同一个 `tele` 的多个窗口、从登录起保持其运行的服务、没有窗口也能收到的通知、对 Telegram 调用的统一额度                                                                                                  |
| `v2.1`              | 脚本化：列出聊天和话题、读取聊天并向其发送消息的命令，支持 JSON 输出；之后是富文本消息                                                                                                                                                   |
| `Backlog`           | 进阶用户与平台：搜索（全文历史搜索、命令面板、聊天内搜索）、语音和圆形视频消息、定时发送、机器人命令和内联键盘、私密聊天、投票、更多 vim 动作、点击通知跳转                                                                              |

工作还按看板上的 **Theme** 字段分类（Security & Reliability、Architecture & Performance、Feature Completeness、Power User & Polish）。

---

<a id="build-from-source"></a>
## 从源码构建

需要 Go 1.26+。这样构建出的二进制文件带有[应用密钥](#app-key)中描述的共用密钥，因此无需注册任何东西就能运行：

```sh
git clone https://github.com/sorokin-vladimir/tele
cd tele
go build -o tele ./cmd/tele/
```

使用自己的应用密钥构建、`nix develop` 环境以及 flake 的 `vendorHash`（英文）：[docs/development.md](docs/development.md)

---

<a id="documentation"></a>
## 文档

- [命令](docs/zh-CN/commands.md)：所有命令及其参数
- [命令行](docs/zh-CN/cli.md)：命令的退出码、JSON 输出以及命名规则
- [配置](docs/zh-CN/configuration.md)：配置文件、设置面板、通知、图片和头像缓存
- [快捷键](docs/zh-CN/keybindings.md)：按上下文列出的全部快捷键，以及如何重新绑定
- [主题](docs/zh-CN/themes.md)：主题槽位、编写自己的主题、所有颜色 token
- [媒体](docs/zh-CN/media.md)：聊天内图片、语音播放、视频、GIF、发送文件
- [应用密钥](docs/zh-CN/app-key.md)：它是什么，以及被 Telegram 封禁时该怎么办
- [Development](docs/development.md)：从源码构建、Nix flake（英文）
- [Contributing](.github/CONTRIBUTING.md)：如何提交修改（英文）

---

<a id="license"></a>
## 许可证

GPL-3.0：可以自由使用和分叉；衍生作品必须保持开源。

---

构建于：

- [gotd/td](https://github.com/gotd/td)
- [bubbletea](https://github.com/charmbracelet/bubbletea)
- [lipgloss](https://github.com/charmbracelet/lipgloss)
- 灵感来自 [lazygit](https://github.com/jesseduffield/lazygit)
