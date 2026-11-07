# 应用密钥

> 本文译自 [app-key.md](../app-key.md)，对应提交 `a8ea9ea`。译文与英文版不一致时，以英文版为准。本译文尚未经过母语者审校。如果发现翻译错误，欢迎[提交 issue](https://github.com/sorokin-vladimir/tele/issues)。

`tele` 通过应用密钥连接 Telegram。应用密钥是一对 `api_id` 和 `api_hash`，它标识的是应用程序，而不是你本人。你不需要为它做任何事：每个构建都自带一个，登录可以直接进行。

<a id="when-telegram-refuses-the-key"></a>
## Telegram 拒绝密钥时

从公开源码编译的构建（homebrew-core、Nix flake、BSD 移植版、你自己的 `go build`）都使用同一个密钥，所有以这种方式构建 `tele` 的人共用它。Telegram 可能会拒绝它认为被过多人共用的密钥。这时 `tele` 会提示 `app key blocked by Telegram`，而不会误报你已退出登录。

有两种解决办法，选更省事的那种即可。

<a id="register-a-key-of-your-own"></a>
### 注册自己的密钥

在 [my.telegram.org](https://my.telegram.org) 申请一个，然后写入 `~/.config/tele/config.yml`。你提供的密钥总是优先于内置密钥：

```yaml
telegram:
  api_id: 123456
  api_hash: "your api hash"
```

两项要么都填，要么都不填。如果配置里只写了其中一项，`tele` 在启动时就会停止：半个密钥标识不了任何东西，而把它和内置密钥的另一半拼在一起，得到的是一个谁都没有签发过的密钥。

设置面板会显示当前生效的是哪个密钥。配置里没有密钥时，API ID 和 API hash 这两行显示的不是数字，而是 `built-in (official build)` 或 `built-in (published)`。

如果想把自己的密钥直接编译进二进制文件，而不是从配置读取，请参阅 [development.md](../development.md#building-with-your-own-app-key)（英文）。

<a id="install-an-official-build"></a>
### 安装官方构建

官方构建自带独立的密钥：Homebrew tap、安装脚本、apt、dnf、apk、Scoop 和 winget 提供的都是由 `tele` 发布流水线构建的二进制文件。

<a id="which-formula-is-which"></a>
## 两个 formula 的区别

Homebrew 可以从两个地方安装 `tele`，它们的区别正在于此。homebrew-core 中的 `brew install tele` 由 Homebrew 从源码构建，因此使用源码构建共用的密钥。`brew install sorokin-vladimir/tap/tele` 安装的是发布流水线构建的二进制文件，带有独立的密钥。

添加 tap 之后，直接运行 `brew install tele` 会选用 tap 中的 formula，而且只用一行 `Warning: tele shadows homebrew/core/tele` 提示，很容易在滚动时错过。所以 README 中两个 formula 都写了全名。
