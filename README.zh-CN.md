# tele

> 本文译自 [README.md](README.md)，对应提交 `bfe07cb`。译文与英文版不一致时，以英文版为准。本译文尚未经过母语者审校。如果发现翻译错误，欢迎[提交 issue](https://github.com/sorokin-vladimir/tele/issues)。

```
  _            _
 | |_    ___  | |   ___
 | __|  / _ \ | |  / _ \
 | |_  |  __/ | | |  __/
  \__|  \___| |_|  \___|
```

> 为键盘操作而生的终端原生 Telegram 客户端。

<p align="center"><a href="README.md">🇬🇧 English</a> · <a href="README.ru.md">🇷🇺 Русский</a></p>

[![Release](https://img.shields.io/github/v/release/sorokin-vladimir/tele)](https://github.com/sorokin-vladimir/tele/releases)
[![Downloads](https://img.shields.io/github/downloads/sorokin-vladimir/tele/total?color=blue)](https://github.com/sorokin-vladimir/tele/releases)
[![License](https://img.shields.io/badge/license-GPL--3.0-green)](LICENSE)
[![Platform](https://img.shields.io/badge/platform-macOS%20%C2%B7%20Linux%20%C2%B7%20Windows-lightgrey)](#installation)

<p align="center">
  <a href="#features">功能</a> •
  <a href="#command-line-and-ai-agents">命令行</a> •
  <a href="#installation">安装</a> •
  <a href="#keybindings">快捷键</a> •
  <a href="#roadmap">路线图</a> •
  <a href="#documentation">文档</a>
</p>

---

![tele demo](./assets/demo.gif)

---

<a id="why-tele"></a>
## 为什么选 `tele`？

Telegram Desktop、网页版和手机应用都是围绕鼠标设计的。如果你的日常工作都在终端里，用着 Neovim、yazi、k9s 或 tmux，切换到图形界面的聊天软件会打断你的节奏。

`tele` 让 Telegram 留在终端里。你可以像用 lazygit 一样用键盘操作它，它能通过 SSH 使用，空闲时只占约 50 MB 内存，而桌面客户端要占几百 MB。作者每天都在用它。

同一个 `tele` 也能在命令行中使用：你，或者你允许运行它的 AI 代理，可以列出聊天、读取聊天并发送消息，而无需打开窗口。

---

<a id="features"></a>
## 功能

- **键盘优先。** vim 风格的导航，以及在消息之间逐条移动的光标。鼠标是可选的。
- **没有窗口也能收到通知。** `tele daemon install` 让账号从登录系统起保持在线。
- **一个账号，多个窗口。** 第二个 `tele` 会连接到正在运行的那个，而不是重新建立连接。
- **聊天、群组、频道和论坛**，支持文件夹、回复、反应、编辑、转发，以及与 Telegram 同步的草稿。
- **富文本消息** 渲染标题、列表、表格、引用和详情块，并保留其布局。
- **聊天内图片**：在 kitty、Ghostty 和 iTerm2 3.7.0+ 中以原画质显示，其他终端中显示为字符块画。
- **语音消息在应用内播放**，带有波形和移动的播放位置。
- **视频、圆形视频和 GIF** 在聊天中显示预览。选中的 GIF 会在原位循环播放。
- **相册。** 用 `u` 添加多个文件，作为一个相册发送。
- **和官方客户端一样的代理**：MTProto（包括 fake-TLS）或 SOCKS5。
- **命令行**，用于读取和发送消息，并附有给 AI 代理的指南。
- **主题**随终端在深色和浅色之间切换。二进制文件中附带八个主题。
- **单个静态二进制文件。** 不需要 TDLib，不依赖 cgo，也不需要安装运行时。

`tele` 不能录制语音或视频，也不能通话：[原因](docs/zh-CN/media.md#recording-is-out-of-scope)。各种媒体类型的详细说明：[docs/zh-CN/media.md](docs/zh-CN/media.md)。

---

<a id="command-line-and-ai-agents"></a>
## 命令行与 AI 代理

命令作用于正在运行的 `tele` 所持有的账号，无论它是窗口还是服务：

```sh
tele chats --unread          # 有未读消息的聊天
tele show me --limit 5       # 收藏夹中的最新消息
tele send me "buy milk"      # 发送，并等待 Telegram 收到
```

给任意命令加上 `--json` 即可输出 JSON Lines。所有命令：[docs/zh-CN/commands.md](docs/zh-CN/commands.md)。退出码和 JSON：[docs/zh-CN/cli.md](docs/zh-CN/cli.md)。

`tele help agents` 会打印给 AI 代理的指南，这份指南对应你安装的 `tele` 版本。`tele` 技能会把代理引向这份指南。

<details>
<summary>安装技能</summary>

| 代理               | 安装技能                                                                                        |
| ------------------ | ----------------------------------------------------------------------------------------------- |
| Claude Code        | `/plugin install tele --marketplace sorokin-vladimir/tele`                                      |
| GitHub Copilot CLI | `copilot plugin marketplace add sorokin-vladimir/tele`，然后 `copilot plugin install tele@tele` |
| OpenAI Codex       | `npx skills add sorokin-vladimir/tele -a codex`                                                 |
| Cursor             | `npx skills add sorokin-vladimir/tele -a cursor`                                                |
| Gemini CLI         | `npx skills add sorokin-vladimir/tele -a gemini-cli`                                            |

Claude Code 低于 2.1.275 时，先添加市场：`claude plugin marketplace add sorokin-vladimir/tele`，然后 `claude plugin install tele@tele`。不带 `-a` 运行 `npx skills add sorokin-vladimir/tele` 时，它会询问要安装到它所知道的哪些代理中，包括 OpenCode 等。

</details>

命令能做到哪一步由 `cli.account_commands` 决定：设为 `read` 时它们可以读取但不能发送，设为 `none` 时它们什么也拿不到。[docs/zh-CN/cli.md](docs/zh-CN/cli.md#keeping-commands-out-of-your-chats)

---

<a id="compared-to-other-terminal-clients"></a>
## 与其他终端客户端对比

几乎所有终端 Telegram 客户端都构建在 Telegram 的 C++ 库 TDLib 之上，要么直接链接，要么通过 Python 调用。`tele` 通过 [gotd/td](https://github.com/gotd/td) 自己实现 MTProto，并且不依赖 cgo 构建。

|                  | `tele`                                         | [tgt](https://github.com/FedericoBruzzone/tgt)                              | [tg](https://github.com/paul-nameless/tg) · [tuigram](https://codeberg.org/Yehoslav/tuigram) |
| ---------------- | ---------------------------------------------- | --------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------- |
| Telegram 后端    | gotd/td，纯 Go 实现的 MTProto                  | TDLib                                                                       | 通过 `python-telegram` 使用 TDLib                                                             |
| 需要安装什么     | 一个静态二进制文件                             | `cargo install` 加 TDLib；语音消息需要 CMake；聊天内图片需要 `chafa`        | Python 3.9+/3.10+ 加 TDLib；`ffmpeg`、`terminal-notifier`、`urlview`、`ranger`/`fzf`          |
| 聊天内图片       | Kitty 图形协议原画质显示，字符块画             | `chafa` 字符块画，需开启可选的构建特性                                      | 通过 mailcap 交给外部查看器                                                                   |
| 无窗口时         | 从登录起作为服务运行；没有窗口也能收到通知     | 只在它的终端打开时运行                                                      | 只在它的终端打开时运行                                                                        |
| Windows          | 二进制文件、Scoop、winget                      | 支持                                                                        | 实际不可用                                                                                    |

`tele` 还有这三者都没有声明支持的功能：反应、聊天文件夹、以相册形式发送、视频、圆形视频和 GIF 的聊天内预览，以及无需外部播放器的语音播放。

`tg` 和 `tuigram` 可以录制语音消息，`tele` 不做这件事（[原因](docs/zh-CN/media.md#recording-is-out-of-scope)）；它们还支持私密聊天，这在 `tele` 的[待办列表](https://github.com/sorokin-vladimir/tele/issues/234)中。

已停止维护的项目没有列入对比：`TelegramTUI`、`arigram`、`tg-tui`、`Telegram-TUI`、`tg9` 和 `ithil`。

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

两个 formula 都特意写了全名。添加 tap 之后，直接运行 `brew install tele` 会选用 tap 中的 formula，而且只用一行 `Warning: tele shadows homebrew/core/tele` 提示，很容易在滚动时错过。两者有一处值得了解的区别，见 [docs/zh-CN/app-key.md](docs/zh-CN/app-key.md#which-formula-is-which)。

<details>
<summary>测试版渠道</summary>

想比每周的稳定版更早用上最新合并的改动，可以从同一个 tap 安装测试版。它是一个独立的 `tele-beta` 二进制文件，有自己的配置和状态（`~/.config/tele-beta`），所以可以和稳定版并存：

```sh
brew tap sorokin-vladimir/tap
brew trust sorokin-vladimir/tap
brew install tele-beta
brew upgrade tele-beta   # 有新测试版时拉取更新
```

测试版来自预发布标签（`vX.Y.Z-beta.N`），在 GitHub 上以预发布版本发布，所以永远不会显示为“最新”版本。用 `tele-beta` 运行。

</details>

<a id="other-platforms"></a>
### 其他平台

<details>
<summary>Linux：二进制文件</summary>

```sh
curl -sL https://github.com/sorokin-vladimir/tele/releases/latest/download/tele-linux-amd64 \
  -o ~/.local/bin/tele && chmod +x ~/.local/bin/tele
```

arm64 上把 `amd64` 换成 `arm64`。

</details>

<details>
<summary>Debian / Ubuntu / Mint：apt</summary>

```sh
echo 'deb [trusted=yes] https://apt.fury.io/sorokin-vladimir/ /' \
  | sudo tee /etc/apt/sources.list.d/tele.list
sudo apt update && sudo apt install tele
```

每个[版本](https://github.com/sorokin-vladimir/tele/releases/latest)也附带了签名的 `.deb` 文件。

</details>

<details>
<summary>Fedora / RHEL / openSUSE：dnf / zypper</summary>

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

每个[版本](https://github.com/sorokin-vladimir/tele/releases/latest)也附带了签名的 `.rpm` 文件。

</details>

<details>
<summary>Alpine：apk</summary>

```sh
echo 'https://alpine.fury.io/sorokin-vladimir/' | sudo tee -a /etc/apk/repositories
sudo apk add --allow-untrusted tele
```

</details>

<details>
<summary>Windows：Scoop、winget 或二进制文件</summary>

通过 [Scoop](https://scoop.sh) 或 [winget](https://learn.microsoft.com/windows/package-manager/)：

```powershell
scoop bucket add tele https://github.com/sorokin-vladimir/scoop-tele
scoop install tele
# 或
winget install sorokin-vladimir.tele
```

或者用 PowerShell 下载可执行文件（以普通用户身份运行）：

```powershell
$dir = "$Env:LOCALAPPDATA\Programs\tele"
New-Item -ItemType Directory -Force -Path $dir | Out-Null
Invoke-WebRequest `
  -Uri "https://github.com/sorokin-vladimir/tele/releases/latest/download/tele-windows-amd64.exe" `
  -OutFile "$dir\tele.exe"
```

arm64 上把 `amd64` 换成 `arm64`。把 `$dir` 加入 `PATH`（或者用完整路径运行 `tele.exe`），然后在任意终端中启动。每个[版本](https://github.com/sorokin-vladimir/tele/releases/latest)也附带一个包含 `tele.exe` 的 `.zip`（`tele_windows_amd64.zip`）。

目前在 Windows 上图片显示为字符块画：[docs/zh-CN/media.md](docs/zh-CN/media.md#photos-on-windows)。

</details>

<details>
<summary>Nix / NixOS</summary>

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

</details>

AUR 和 Snap 软件包已在计划中，但尚未发布。

---

<a id="getting-started"></a>
## 快速上手

运行 `tele`，用手机上的 Telegram 扫描它显示的二维码，或者用手机号、Telegram 发给你的验证码以及两步验证密码（如果设置了）登录。之后 `tele` 会询问一次是否作为服务继续运行。

首次启动、服务、应用密钥以及从 1.11 升级：[docs/zh-CN/getting-started.md](docs/zh-CN/getting-started.md)

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

所有配置都在 `~/.config/tele/config.yml` 中。按 `,` 打开设置面板：其中列出所有设置项，修改会写入同一个文件，并保留你的注释。[docs/zh-CN/configuration.md](docs/zh-CN/configuration.md)

`tele` 同时持有一个深色主题和一个浅色主题，并随终端背景的变化在它们之间切换。二进制文件中附带八个主题，你自己的主题是只设置它要改的颜色的文件。[docs/zh-CN/themes.md](docs/zh-CN/themes.md)

每个按键都能在配置的 `keybindings:` 一节中按上下文重新绑定，包括组合键序列。[docs/zh-CN/keybindings.md](docs/zh-CN/keybindings.md#customizing-keybindings)

---

<a id="roadmap"></a>
## 路线图

计划中的工作都在公开的[**项目看板**](https://github.com/users/sorokin-vladimir/projects/2)上，按[里程碑](https://github.com/sorokin-vladimir/tele/milestones)分组。里程碑对应次版本（`v1.11`、`v2.0`……）；补丁版本在修复完成后于该版本线内陆续发布。

| 版本                | 重点                                                                                                                                                                                                                                     |
| ------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `v1.11` _(已完成)_  | 设置与用户：应用内设置面板、用户资料面板、头像、内置于二进制文件的主题，以及遗留的分发和审查问题                                                                                                                                         |
| `v2.0` _(已完成)_   | Telegram 连接的唯一持有者：连接到同一个 `tele` 的多个窗口、从登录起保持其运行的服务、没有窗口也能收到的通知、对 Telegram 调用的统一额度；2.0.1 加入了列出聊天和话题、读取聊天并向其发送消息的命令，以及给 AI 代理的指南                 |
| `v2.1`              | 渲染与布局：富文本消息（表格、标题、列表）、按实际宽度显示含 emoji 的名字、让聚焦的面板占满整个屏幕、撰写消息时选择 emoji                                                                                                               |
| `v2.2`              | 更多命令行功能：登录、搜索、发送文件和反应、随新消息到达持续跟踪聊天                                                                                                                                                                     |
| `Backlog`           | 进阶用户与平台：搜索（全文历史搜索、命令面板、聊天内搜索）、定时发送、机器人命令和内联键盘、私密聊天、投票、更多 vim 动作、点击通知跳转                                                                              |

工作还按看板上的 **Theme** 字段分类（Security & Reliability、Architecture & Performance、Feature Completeness、Power User & Polish）。

---

<a id="build-from-source"></a>
## 从源码构建

需要 Go 1.26+。这样构建出的二进制文件带有 [docs/zh-CN/app-key.md](docs/zh-CN/app-key.md) 中描述的共用密钥，因此无需注册任何东西就能运行：

```sh
git clone https://github.com/sorokin-vladimir/tele
cd tele
go build -o tele ./cmd/tele/
```

使用自己的应用密钥构建、`nix develop` 环境以及 flake 的 `vendorHash`（英文）：[docs/development.md](docs/development.md)

---

<a id="documentation"></a>
## 文档

- [快速上手](docs/zh-CN/getting-started.md)：首次启动、服务、应用密钥、从 1.11 升级
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
