# tele

> Перевод [README.md](README.md) на коммит `2556c34`. Если перевод расходится с английской версией, верна английская.

```
  _            _
 | |_    ___  | |   ___
 | __|  / _ \ | |  / _ \
 | |_  |  __/ | | |  __/
  \__|  \___| |_|  \___|
```

> Клиент Telegram для терминала, созданный для работы с клавиатуры.

<p align="center"><a href="README.md">🇬🇧 English</a> · <a href="README.zh-CN.md">🇨🇳 简体中文</a></p>

[![Release](https://img.shields.io/github/v/release/sorokin-vladimir/tele)](https://github.com/sorokin-vladimir/tele/releases)
[![Downloads](https://img.shields.io/github/downloads/sorokin-vladimir/tele/total?color=blue)](https://github.com/sorokin-vladimir/tele/releases)
[![License](https://img.shields.io/badge/license-GPL--3.0-green)](LICENSE)
[![Platform](https://img.shields.io/badge/platform-macOS%20%C2%B7%20Linux%20%C2%B7%20Windows-lightgrey)](#installation)

<p align="center">
  <a href="#features">Возможности</a> •
  <a href="#command-line-and-ai-agents">Командная строка</a> •
  <a href="#installation">Установка</a> •
  <a href="#keybindings">Клавиши</a> •
  <a href="#roadmap">Планы</a> •
  <a href="#documentation">Документация</a>
</p>

---

![tele demo](./assets/demo.gif)

---

<a id="why-tele"></a>
## Зачем `tele`?

Telegram Desktop, веб-клиент и мобильные приложения рассчитаны на мышь.
Если вы живёте в терминале и пользуетесь Neovim, yazi, k9s или tmux, переход в графический мессенджер сбивает с ритма.

`tele` оставляет Telegram в терминале.
Им управляют с клавиатуры, как lazygit, и он работает по SSH.
На старте фоновый сервис и окно занимают около 50 МБ памяти каждый, а когда накопится кэш медиа, больше, тогда как десктопным клиентам нужно несколько сотен.
Автор пользуется им каждый день.

Тот же `tele` отвечает и в командной строке: вы или ИИ-агент, которому вы разрешили его запускать, можете посмотреть список чатов, прочитать чат и отправить сообщение, не открывая окна.

---

<a id="features"></a>
## Возможности

- **Всё с клавиатуры.**
  Навигация в духе vim и курсор, который переходит от сообщения к сообщению.
  Мышь необязательна.
- **Уведомления без открытого окна.**
  `tele daemon install` держит аккаунт на связи с момента входа в систему.
- **Один аккаунт, много окон.**
  Второй `tele` подключается к уже работающему, а не соединяется заново.
- **Чаты, группы, каналы и форумы** с папками, ответами, реакциями, правками, пересылкой и черновиками, синхронизированными с Telegram.
- **Фото прямо в чате** в полном качестве в kitty, Ghostty и iTerm2 3.7.0+, везде остальном блочной графикой.
- **Голосовые проигрываются в приложении**, с волной и движущейся позицией.
- **Видео, кружки и GIF** с превью прямо в чате.
  Выбранная GIF проигрывается по кругу на своём месте.
- **Альбомы.**
  Добавьте несколько файлов через `u` и отправьте их одним альбомом.
- **Прокси**, как в официальных клиентах: MTProto, включая fake-TLS, или SOCKS5.
- **Командная строка** для чтения и отправки и инструкция для ИИ-агентов.
- **Темы**, которые переключаются между тёмной и светлой вслед за терминалом.
  Восемь поставляются в бинарнике.
- **Один статический бинарник.**
  Без TDLib, без cgo, без среды выполнения.

`tele` не записывает голосовые и видео и не звонит: [почему](docs/ru/media.md#recording-is-out-of-scope).
Все типы медиа подробно: [docs/ru/media.md](docs/ru/media.md).

---

<a id="command-line-and-ai-agents"></a>
## Командная строка и ИИ-агенты

Команды работают с аккаунтом, который держит запущенный `tele`: окно или сервис.

```sh
tele chats --unread          # чаты с непрочитанными сообщениями
tele show me --limit 5       # последние сообщения в Избранном
tele send me "buy milk"      # отправить и дождаться, пока Telegram его примет
```

С `--json` любая из них выводит JSON Lines.
Все команды: [docs/ru/commands.md](docs/ru/commands.md).
Коды завершения и JSON: [docs/ru/cli.md](docs/ru/cli.md).

`tele help agents` печатает инструкцию для ИИ-агентов, написанную для установленного у вас `tele`.
Навык `tele` отправляет агента к ней.

<details>
<summary>Как установить навык</summary>

| Агент              | Как установить навык                                                                             |
| ------------------ | ------------------------------------------------------------------------------------------------ |
| Claude Code        | `/plugin install tele --marketplace sorokin-vladimir/tele`                                       |
| GitHub Copilot CLI | `copilot plugin marketplace add sorokin-vladimir/tele`, затем `copilot plugin install tele@tele` |
| OpenAI Codex       | `npx skills add sorokin-vladimir/tele -a codex`                                                  |
| Cursor             | `npx skills add sorokin-vladimir/tele -a cursor`                                                 |
| Gemini CLI         | `npx skills add sorokin-vladimir/tele -a gemini-cli`                                             |

В Claude Code старше 2.1.275 сначала добавьте маркетплейс: `claude plugin marketplace add sorokin-vladimir/tele`, затем `claude plugin install tele@tele`.
`npx skills add sorokin-vladimir/tele` без `-a` спросит, в каких из известных ему агентов поставить навык, в том числе в OpenCode и других.

</details>

Насколько далеко могут зайти команды, решает `cli.account_commands`: с `read` они читают, но не отправляют, с `none` не получают ничего.
[docs/ru/cli.md](docs/ru/cli.md#keeping-commands-out-of-your-chats)

---

<a id="compared-to-other-terminal-clients"></a>
## Сравнение с другими терминальными клиентами

Почти все терминальные клиенты Telegram построены на TDLib, библиотеке Telegram на C++, подключённой напрямую или через Python.
`tele` сам говорит на MTProto через [gotd/td](https://github.com/gotd/td) и собирается без cgo.

|                  | `tele`                                                   | [tgt](https://github.com/FedericoBruzzone/tgt)                                        | [tg](https://github.com/paul-nameless/tg) · [tuigram](https://codeberg.org/Yehoslav/tuigram) |
| ---------------- | -------------------------------------------------------- | ------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------- |
| Связь с Telegram | gotd/td: MTProto на чистом Go                            | TDLib                                                                                 | TDLib через `python-telegram`                                                                 |
| Что ставить      | один статический бинарник                                | `cargo install` плюс TDLib; CMake для голосовых; `chafa` для фото в чате              | Python 3.9+/3.10+ плюс TDLib; `ffmpeg`, `terminal-notifier`, `urlview`, `ranger`/`fzf`        |
| Фото в чате      | протокол графики Kitty в полном качестве, блочная графика | блочная графика `chafa`, за необязательным флагом сборки                             | внешний просмотрщик через mailcap                                                             |
| Без окна         | сервис с момента входа в систему; уведомления без окна   | работает, только пока открыт его терминал                                             | работает, только пока открыт его терминал                                                     |
| Windows          | бинарник, Scoop, winget                                  | поддерживается                                                                        | на практике неприменим                                                                        |

Ещё у `tele` есть то, чего не заявляет ни один из трёх: реакции, папки чатов, отправка альбомов, превью прямо в чате для видео, кружков и GIF и воспроизведение голосовых без внешнего плеера.

`tg` и `tuigram` записывают голосовые, чего в `tele` нет ([почему](docs/ru/media.md#recording-is-out-of-scope)), и поддерживают секретные чаты, которые есть в [бэклоге](https://github.com/sorokin-vladimir/tele/issues/234).

Заброшенные проекты в сравнение не вошли: `TelegramTUI`, `arigram`, `tg-tui`, `Telegram-TUI`, `tg9` и `ithil`.

---

<a id="installation"></a>
## Установка

<a id="any-unix-macos--linux--freebsd--openbsd--netbsd---one-liner"></a>
### Любой Unix (macOS · Linux · FreeBSD · OpenBSD · NetBSD): одной командой

```sh
curl -sL https://raw.githubusercontent.com/sorokin-vladimir/tele/main/scripts/install.sh | sh
```

Скрипт определяет ОС и архитектуру и ставит подходящий бинарник из последнего релиза.
Варианты:

```sh
# Последняя бета (пререлиз), ставится как `tele-beta` и уживается со стабильной:
curl -sL https://raw.githubusercontent.com/sorokin-vladimir/tele/main/scripts/install.sh | sh -s -- --beta

# Конкретная версия или свой каталог установки:
curl -sL .../install.sh | sh -s -- --version v1.9.0
curl -sL .../install.sh | PREFIX="$HOME/.local/bin" sh
```

На FreeBSD системные уведомления идут через терминал (только в терминалах, которые это поддерживают), а для воспроизведения звука нужен работающий сервер PulseAudio/PipeWire.
Без них соответствующая функция отключается без ошибок.

<a id="macos--linux---homebrew"></a>
### macOS / Linux: Homebrew

```sh
brew install tele  # из homebrew/core
```

Или из собственного tap `tele`:

```sh
brew tap sorokin-vladimir/tap
brew trust sorokin-vladimir/tap
brew install sorokin-vladimir/tap/tele
```

Обе формулы записаны полностью намеренно.
Если tap подключён, простой `brew install tele` выбирает формулу из tap и сообщает об этом только строкой `Warning: tele shadows homebrew/core/tele`, которую легко пропустить.
Формулы отличаются в одном, о чём стоит знать; это описано в [docs/ru/app-key.md](docs/ru/app-key.md#which-formula-is-which).

<details>
<summary>Канал бет</summary>

Чтобы получать последние влитые изменения раньше еженедельного стабильного релиза, поставьте пакет беты из того же tap.
Это отдельный бинарник `tele-beta` со своими конфигом и состоянием (`~/.config/tele-beta`), поэтому он уживается со стабильной версией:

```sh
brew tap sorokin-vladimir/tap
brew trust sorokin-vladimir/tap
brew install tele-beta
brew upgrade tele-beta   # подтягивать новые беты по мере выхода
```

Беты собираются из тегов пререлизов (`vX.Y.Z-beta.N`) и публикуются на GitHub как пререлизы, поэтому никогда не становятся «последним» релизом.
Запускается командой `tele-beta`.

</details>

<a id="other-platforms"></a>
### Другие платформы

<details>
<summary>Linux: бинарник</summary>

```sh
curl -sL https://github.com/sorokin-vladimir/tele/releases/latest/download/tele-linux-amd64 \
  -o ~/.local/bin/tele && chmod +x ~/.local/bin/tele
```

Для arm64 замените `amd64` на `arm64`.

</details>

<details>
<summary>Debian / Ubuntu / Mint: apt</summary>

```sh
echo 'deb [trusted=yes] https://apt.fury.io/sorokin-vladimir/ /' \
  | sudo tee /etc/apt/sources.list.d/tele.list
sudo apt update && sudo apt install tele
```

Этот репозиторий подключается без проверки подписи: apt ставит всё, что он отдаёт ([#342](https://github.com/sorokin-vladimir/tele/issues/342)).
Чтобы поставить проверенный пакет, скачайте `.deb` из [последнего релиза](https://github.com/sorokin-vladimir/tele/releases/latest), проверьте его, как описано в разделе [Проверка загрузки](#checking-a-download), и выполните `sudo apt install ./tele_<версия>_amd64.deb`.

</details>

<details>
<summary>Fedora / RHEL / openSUSE: dnf / zypper</summary>

```sh
sudo tee /etc/yum.repos.d/tele.repo <<'EOF'
[tele]
name=tele
baseurl=https://yum.fury.io/sorokin-vladimir/
enabled=1
gpgcheck=0
EOF
sudo dnf install tele   # или: sudo zypper install tele
```

Этот репозиторий подключается без проверки подписи: dnf и zypper ставят всё, что он отдаёт ([#342](https://github.com/sorokin-vladimir/tele/issues/342)).
Чтобы поставить проверенный пакет, скачайте `.rpm` из [последнего релиза](https://github.com/sorokin-vladimir/tele/releases/latest), проверьте его, как описано в разделе [Проверка загрузки](#checking-a-download), и выполните `sudo dnf install ./tele-<версия>-1.x86_64.rpm`.

</details>

<details>
<summary>Alpine: apk</summary>

```sh
echo 'https://alpine.fury.io/sorokin-vladimir/' | sudo tee -a /etc/apk/repositories
sudo apk add --allow-untrusted tele
```

`--allow-untrusted` выключает проверку подписи в apk: пакет пока не подписан ([#342](https://github.com/sorokin-vladimir/tele/issues/342)).
Чтобы поставить проверенный пакет, скачайте `.apk` из [последнего релиза](https://github.com/sorokin-vladimir/tele/releases/latest), проверьте его, как описано в разделе [Проверка загрузки](#checking-a-download), и выполните `sudo apk add --allow-untrusted ./tele_<версия>_x86_64.apk`.

</details>

<details>
<summary>Windows: Scoop, winget или бинарник</summary>

Через [Scoop](https://scoop.sh) или [winget](https://learn.microsoft.com/windows/package-manager/):

```powershell
scoop bucket add tele https://github.com/sorokin-vladimir/scoop-tele
scoop install tele
# или
winget install sorokin-vladimir.tele
```

Или скачайте исполняемый файл через PowerShell (от имени обычного пользователя):

```powershell
$dir = "$Env:LOCALAPPDATA\Programs\tele"
New-Item -ItemType Directory -Force -Path $dir | Out-Null
Invoke-WebRequest `
  -Uri "https://github.com/sorokin-vladimir/tele/releases/latest/download/tele-windows-amd64.exe" `
  -OutFile "$dir\tele.exe"
```

Для arm64 замените `amd64` на `arm64`.
Добавьте `$dir` в `PATH` (или запускайте `tele.exe` по полному пути) и запускайте из любого терминала.
Архив `.zip` с `tele.exe` (`tele_windows_amd64.zip`) также приложен к каждому [релизу](https://github.com/sorokin-vladimir/tele/releases/latest).

Фото на Windows пока рисуются блочной графикой: [docs/ru/media.md](docs/ru/media.md#photos-on-windows).

</details>

<details>
<summary>Nix / NixOS</summary>

У `tele` есть flake.
Попробовать без установки:

```sh
nix run github:sorokin-vladimir/tele
```

Или поставить в свой профиль:

```sh
nix profile install github:sorokin-vladimir/tele
```

Чтобы подключить его как вход своего flake (например, конфигурации NixOS):

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

Затем в конфигурации:

```nix
environment.systemPackages = [
  inputs.tele.packages.${pkgs.system}.default
];
```

</details>

Пакеты для AUR и Snap запланированы, но пока не опубликованы.

<a id="checking-a-download"></a>
### Проверка загрузки

К каждому релизу приложен `checksums.txt` с SHA-256 всех бинарников, архивов и пакетов, подписанный в release workflow через [Sigstore](https://www.sigstore.dev/).
Чтобы проверить скачанный файл, положите рядом с ним `checksums.txt` и `checksums.txt.sigstore.json` из того же релиза и выполните:

```sh
cosign verify-blob --bundle checksums.txt.sigstore.json \
  --certificate-identity-regexp '^https://github.com/sorokin-vladimir/tele/\.github/workflows/release\.yml@refs/tags/v' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  checksums.txt
sha256sum --ignore-missing -c checksums.txt   # macOS: shasum -a 256 --ignore-missing -c checksums.txt
```

Первая команда показывает, что `checksums.txt` сделан release workflow tele, вторая — что ваш файл с ним совпадает.

---

<a id="getting-started"></a>
## Первые шаги

Запустите `tele` и отсканируйте показанный QR-код в Telegram на телефоне или войдите по номеру телефона, коду, который пришлёт Telegram, и паролю двухэтапной проверки, если он есть.
Затем `tele` один раз спросит, продолжать ли работать как сервис.

Первый запуск, сервис, ключ приложения и переход с 1.11: [docs/ru/getting-started.md](docs/ru/getting-started.md)

---

<a id="keybindings"></a>
## Клавиши

| Клавиша             | Действие                                          |
| ------------------- | ------------------------------------------------- |
| `j` / `k`           | Перемещаться по чатам или выбирать следующее / предыдущее сообщение |
| `ctrl+j` / `ctrl+k` | Прокручивать сообщения                            |
| `i`                 | Написать сообщение                                |
| `r`                 | Ответить                                          |
| `e` / `d`           | Изменить / удалить сообщение                      |
| `t`                 | Поставить реакцию                                 |
| `f`                 | Переслать сообщение                               |
| `u`                 | Прикрепить файл (повторите, чтобы отправить альбом) |
| `o`                 | Открыть / посмотреть медиа (фото в просмотрщике, видео прямо в чате) |
| `O`                 | Открыть видео во внешнем плеере                   |
| `p`                 | Воспроизвести голосовое в приложении              |
| `s`                 | Скачать выбранный файл                            |
| `/`                 | Поиск чатов                                       |
| `space`             | Контекстное меню сообщения                        |
| `0` / `1` / `2`     | Перейти к панели                                  |
| `,`                 | Настройки                                         |
| `g L`               | Выйти из аккаунта                                 |
| `?`                 | Клавиатурные сокращения                           |
| `q`                 | Выйти                                             |

Полный список: [docs/ru/keybindings.md](docs/ru/keybindings.md)

---

<a id="configuration"></a>
## Настройка

Всё хранится в `~/.config/tele/config.yml`.
Клавиша `,` открывает окно настроек: в нём все настройки, а изменения записываются в тот же файл с сохранением ваших комментариев.
[docs/ru/configuration.md](docs/ru/configuration.md)

`tele` держит тёмную и светлую тему и переключается между ними вслед за фоном терминала.
Восемь тем поставляются в бинарнике, а ваши собственные лежат файлами и задают только те цвета, которые меняют.
[docs/ru/themes.md](docs/ru/themes.md)

Любую клавишу можно переназначить в каждом контексте, включая аккорды, в разделе `keybindings:` конфига.
[docs/ru/keybindings.md](docs/ru/keybindings.md#customizing-keybindings)

---

<a id="roadmap"></a>
## Планы

Запланированная работа ведётся на открытой [**доске проекта**](https://github.com/users/sorokin-vladimir/projects/2) и сгруппирована по [вехам](https://github.com/sorokin-vladimir/tele/milestones).
Вехи соответствуют минорным версиям (`v1.11`, `v2.0`, …); патч-релизы выходят внутри версии по мере исправлений.

| Релиз               | Чему посвящён                                                                                                                                                                                                                            |
| ------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `v1.11` _(готово)_  | Настройки и люди: окно настроек в приложении, окно профиля, аватары, темы внутри бинарника, а также оставшиеся пробелы в распространении и аудите                                                                                          |
| `v2.0` _(готово)_   | Один владелец соединения с Telegram: окна, которые подключаются к одному `tele`, сервис, который поддерживает его работу с момента входа в систему, уведомления без открытого окна, один бюджет на обращения к Telegram; в 2.0.1 команды, которые выводят список чатов и тем, читают чат и отправляют в него, и инструкция для ИИ-агентов |
| `v2.1`              | Отрисовка и раскладка: форматированные сообщения (таблицы, заголовки, списки), имена с эмодзи в их настоящую ширину, панель в фокусе на весь экран, выбор эмодзи при наборе сообщения                                                      |
| `v2.2`              | Больше командной строки: вход в аккаунт, поиск, отправка файлов и реакций, слежение за чатом по мере прихода сообщений                                                                                                                   |
| `Backlog`           | Для опытных пользователей и платформы: поиск (полнотекстовый по истории, палитра команд, поиск внутри чата), отложенная отправка, команды ботов и встроенные клавиатуры, секретные чаты, опросы, расширенные движения vim, переход по уведомлению |

Работа также разбита по направлениям (Security & Reliability, Architecture & Performance, Feature Completeness, Power User & Polish) в поле **Theme** на доске.

---

<a id="build-from-source"></a>
## Сборка из исходников

Нужен Go 1.26+.
Получившийся бинарник содержит общий ключ приложения, описанный в [docs/ru/app-key.md](docs/ru/app-key.md), поэтому работает без регистрации:

```sh
git clone https://github.com/sorokin-vladimir/tele
cd tele
go build -o tele ./cmd/tele/
```

Сборка со своим ключом приложения, оболочка `nix develop` и `vendorHash` во flake (на английском): [docs/development.md](docs/development.md)

---

<a id="documentation"></a>
## Документация

- [Первые шаги](docs/ru/getting-started.md): первый запуск, сервис, ключ приложения, переход с 1.11
- [Команды](docs/ru/commands.md): все команды и их флаги
- [Командная строка](docs/ru/cli.md): с каким кодом завершаются команды, их JSON и как они названы
- [Настройка](docs/ru/configuration.md): файл конфигурации, окно настроек, уведомления, кэши фото и аватаров
- [Клавиши](docs/ru/keybindings.md): все клавиши по контекстам и как их переназначить
- [Темы](docs/ru/themes.md): слоты тем, как написать свою, все цветовые токены
- [Медиа](docs/ru/media.md): фото в чате, воспроизведение голосовых, видео, GIF, отправка файлов
- [Ключ приложения](docs/ru/app-key.md): что это такое и что делать, если Telegram его заблокировал
- [Development](docs/development.md): сборка из исходников, Nix flake (на английском)
- [Contributing](.github/CONTRIBUTING.md): как предложить изменение (на английском)

---

<a id="license"></a>
## Лицензия

GPL-3.0: можно свободно использовать и форкать; производные работы должны оставаться открытыми.

---

Сделано с помощью:

- [gotd/td](https://github.com/gotd/td)
- [bubbletea](https://github.com/charmbracelet/bubbletea)
- [lipgloss](https://github.com/charmbracelet/lipgloss)
- вдохновлено [lazygit](https://github.com/jesseduffield/lazygit)
