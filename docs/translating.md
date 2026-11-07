# Translating the documentation

The documents a person reads to use `tele` are translated into Russian and Simplified Chinese.
The English text is the original: a translation follows it and never adds to it, and where the two disagree, the English one is right.

## What is translated

| English                 | Russian                    | Simplified Chinese            |
| ----------------------- | -------------------------- | ----------------------------- |
| `README.md`             | `README.ru.md`             | `README.zh-CN.md`             |
| `docs/README.md`        | `docs/ru/README.md`        | `docs/zh-CN/README.md`        |
| `docs/<name>.md`        | `docs/ru/<name>.md`        | `docs/zh-CN/<name>.md`        |

The `docs/<name>.md` row covers `app-key`, `cli`, `commands`, `configuration`, `getting-started`, `keybindings`, `media` and `themes`.
The documents for those working on `tele` (this one, `development.md`, `gotd-workarounds.md`, `CONTRIBUTING.md`, the changelog) stay in English, and a translation links the English one.

`internal/docs` holds the tests that keep this true: every document in the list has both translations, and nothing else lives in `docs/ru` or `docs/zh-CN`.

## The rules a translation keeps

**It says what it was made from.**
Its first line under the title is a quote that links the English document and names the commit it was translated at, in backticks:

```markdown
> Перевод [configuration.md](../configuration.md) на коммит `ba129a5`. Если перевод расходится с английской версией, верна английская.
```

The Simplified Chinese line also says that no native speaker has reviewed the translation yet, and asks a reader who notices a mistake to open an issue.
Drop those sentences once one has.

The link and the commit stay on one line.
When you bring a translation up to date, translate what changed since that commit (`git diff <commit> -- docs/configuration.md`) and move the commit forward.

**Its sections keep the English anchors.**
A link such as `configuration.md#running-without-a-window` must work in every language, and GitHub builds an anchor from the heading's own text.
So every heading below the title has the English heading's anchor on the line above it:

```markdown
<a id="running-without-a-window"></a>
## Работа без окна
```

**Its links stay inside its language.**
A translation links the other translations in the same language, and the English document where there is no translation.
The README of each language and `docs/README.md` of each language link their counterparts in every other language, under the title, each with its flag: 🇬🇧 English, 🇷🇺 Русский, 🇨🇳 简体中文.

**What is typed stays as it is.**
Commands, flags, config keys and their values, key names, file paths, code blocks, the config examples (comments included, since they repeat the file `tele` writes), the names of themes and tokens, and the text `tele` prints.
The interface is in English, so a translation quotes it in English.

The tests check the files, the first line, the links and the anchors.
They do not check that a translation says what the English says now: that is the commit in the first line, and the person changing the English text.

## Words

The same thing is called the same everywhere, in one language and across the documents.
A term that is not here is translated the way Telegram's own app in that language says it.

| English                        | Russian                              | Simplified Chinese |
| ------------------------------ | ------------------------------------ | ------------------ |
| window                         | окно                                 | 窗口               |
| the `tele` that holds the account | `tele`, который держит аккаунт    | 持有账号的 `tele`  |
| service                        | сервис                               | 服务               |
| service manager                | менеджер сервисов                    | 服务管理器         |
| command                        | команда                              | 命令               |
| state directory                | каталог состояния                    | 状态目录           |
| config file                    | файл конфигурации                    | 配置文件           |
| setting                        | настройка                            | 设置项             |
| settings overlay               | окно настроек                        | 设置面板           |
| toast                          | тост                                 | 弹出提示           |
| desktop notification           | системное уведомление                | 桌面通知           |
| status bar                     | строка состояния                     | 状态栏             |
| composer                       | поле ввода                           | 输入框             |
| chat list                      | список чатов                         | 聊天列表           |
| folder                         | папка                                | 文件夹             |
| forum, topic                   | форум, тема                          | 论坛，话题         |
| bubble                         | пузырь                               | 气泡               |
| round video                    | кружок                               | 圆形视频           |
| mute, unmute                   | выключить, включить уведомления      | 静音，取消静音     |
| log in, log out                | войти, выйти                         | 登录，退出登录     |
| account                        | аккаунт                              | 账号               |
| app key                        | ключ приложения                      | 应用密钥           |
| built-in key                   | встроенный ключ                      | 内置密钥           |
| official build, source build   | официальная сборка, сборка из исходников | 官方构建，源码构建 |
| theme, slot, token             | тема, слот, токен                    | 主题，槽位，token  |
| base (of a theme)              | база                                 | 基础主题（base）   |
| built-in theme                 | встроенная тема                      | 内置主题           |
| bundled theme                  | поставляемая тема                    | 随附主题           |
| canvas, surface                | холст, поверхность                   | 画布，表面         |
| proxy, direct                  | прокси, напрямую                     | 代理，直连         |
| secret (of a proxy)            | секрет                               | secret             |
| flood wait                     | flood wait, ожидание                 | flood wait，等待   |
| reload                         | перечитать                           | 重新加载           |
| normal mode, insert mode       | обычный режим, режим ввода           | 普通模式，插入模式 |
| context (of a key)             | контекст                             | 上下文             |
| chord                          | аккорд                               | 组合键序列         |
