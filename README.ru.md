# tele

> Перевод [README.md](README.md) на коммит `ba129a5`. Если перевод расходится с английской версией, верна английская.

```
  _            _
 | |_    ___  | |   ___
 | __|  / _ \ | |  / _ \
 | |_  |  __/ | | |  __/
  \__|  \___| |_|  \___|
```

> Клиент Telegram для терминала, созданный для работы с клавиатуры.

<p align="center"><a href="README.md">🇬🇧 English</a> · <a href="README.zh-CN.md">🇨🇳 简体中文</a></p>

[![Go](https://img.shields.io/badge/go-1.26+-blue)](https://go.dev)
[![License](https://img.shields.io/badge/license-GPL--3.0-green)](LICENSE)
[![Release](https://img.shields.io/github/v/release/sorokin-vladimir/tele)](https://github.com/sorokin-vladimir/tele/releases)
[![Downloads](https://img.shields.io/github/downloads/sorokin-vladimir/tele/total?color=blue)](https://github.com/sorokin-vladimir/tele/releases)
[![Stars](https://img.shields.io/github/stars/sorokin-vladimir/tele?style=flat)](https://github.com/sorokin-vladimir/tele/stargazers)
[![Last commit](https://img.shields.io/github/last-commit/sorokin-vladimir/tele)](https://github.com/sorokin-vladimir/tele/commits/main)
[![Platform](https://img.shields.io/badge/platform-macOS%20%C2%B7%20Linux%20%C2%B7%20Windows-lightgrey)](#installation)

<p align="center">
  <a href="#features">Возможности</a> •
  <a href="#installation">Установка</a> •
  <a href="#why-tele">Зачем tele?</a> •
  <a href="#compared-to-other-terminal-clients">Сравнение с другими TUI-клиентами</a> •
  <a href="#keybindings">Клавиши</a> •
  <a href="#roadmap">Планы</a> •
  <a href="#documentation">Документация</a>
</p>

---

![tele demo](./assets/demo.gif)

> **Состояние:** полноценный клиент Telegram для терминала, которым автор пользуется каждый день: чаты, группы, каналы и форумы, медиа, реакции, папки и уведомления, которые приходят, даже когда окно не открыто. То, чего он пока не умеет, есть в [планах](#roadmap).

---

<a id="why-tele"></a>
## Зачем `tele`?

Telegram Desktop, веб-клиент и мобильные приложения рассчитаны в первую очередь на мышь.

Если вы живёте в терминале и пользуетесь Neovim, yazi, k9s или tmux, переход в графический мессенджер сбивает с ритма.

`tele` оставляет вас в терминале.

Он сделан для:

- навигации с клавиатуры
- быстрого переключения между чатами
- работы по SSH и на удалённых машинах
- переписки без отвлечений

Если вам удобны инструменты вроде lazygit, будет удобен и `tele`.

К тому же он лёгкий: в простое обычно около 50 МБ RSS против нескольких сотен МБ у десктопных клиентов.

---

<a id="compared-to-other-terminal-clients"></a>
## Сравнение с другими терминальными клиентами

Почти все терминальные клиенты Telegram построены на TDLib, библиотеке Telegram
на C++, подключённой напрямую или через среду Python. `tele` сам говорит на
MTProto через [gotd/td](https://github.com/gotd/td) и собирается без cgo,
поэтому не нужно скачивать или собирать библиотеку на C++, держать на машине
интерпретатор и ставить цепочку дополнительных программ, прежде чем приложением
можно будет пользоваться полностью.

|                  | `tele`                                                           | [tgt](https://github.com/FedericoBruzzone/tgt)                                                                  | [tg](https://github.com/paul-nameless/tg) · [tuigram](https://codeberg.org/Yehoslav/tuigram)                              |
| ---------------- | ---------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------- |
| Язык             | Go                                                               | Rust                                                                                                            | Python                                                                                                                    |
| Связь с Telegram | gotd/td: MTProto на чистом Go                                    | TDLib                                                                                                           | TDLib через `python-telegram`                                                                                             |
| Что ставить      | один статический бинарник                                        | `cargo install` плюс скачанная или собранная TDLib; CMake для голосовых; системный `chafa` для фото в чате      | среда Python 3.9+/3.10+ плюс TDLib; `ffmpeg`, `terminal-notifier`, `urlview`, `ranger`/`fzf` для всех возможностей        |
| Пакеты           | brew, apt, dnf, zypper, apk, nix, scoop, winget, подписанные deb/rpm | crates.io, AUR, nix, Docker                                                                                 | PyPI, AUR, Docker                                                                                                         |
| Фото в чате      | протокол графики Kitty в полном качестве, иначе блочная ANSI-графика | блочная графика `chafa`, за необязательным флагом сборки                                                    | передаются внешнему просмотрщику через mailcap                                                                            |
| Windows          | бинарник, Scoop, winget                                          | поддерживается                                                                                                  | на практике неприменим                                                                                                    |
| Без окна         | сервис с момента входа в систему; уведомления без открытого окна | работает, только пока открыт его терминал; системные уведомления не заявлены                                    | работает, только пока открыт его терминал; уведомляет через внешнюю команду (`terminal-notifier`, `dunst`)                |
| Частота релизов  | еженедельные стабильные релизы и отдельный канал бет             | последний релиз `v1.0.0-rc1`; последние коммиты обновляют зависимости                                         | `tg` выходит рывками с перерывами в месяцы; `tuigram` сделан как форк форка                                               |

Помимо упаковки, `tele` поддерживает то в современном Telegram, чего ни один из
трёх не заявляет: реакции, папки чатов, отправку альбомов и превью прямо в чате
для видео, кружков и GIF. Голосовые проигрываются внутри приложения, с волной и
движущейся позицией, без внешнего плеера и без сборки декодера Opus через CMake. Чаты открываются мгновенно из локальной истории в
SQLite, темы поставляются в виде восьми встроенных палитр, которые переключаются
между тёмной и светлой вслед за фоном терминала (а `tele theme check` проверяет
контраст), и любую клавишу можно переназначить в каждом контексте, включая
аккорды.

Где они впереди: `tg` и `tuigram` умеют записывать голосовые (через `ffmpeg`)
и поддерживают секретные чаты. Первое для `tele` не предусмотрено
([почему](docs/ru/media.md#recording-is-out-of-scope)), второе есть в
[бэклоге](https://github.com/sorokin-vladimir/tele/issues/234). Полнотекстовый
поиск по сообщениям тоже пока в [планах](#roadmap).

Полностью заброшенные проекты в таблицу не вошли: `TelegramTUI` (помечен
устаревшим, последние изменения в 2022), `arigram` (в архиве), `tg-tui` (2018),
`Telegram-TUI`, `tg9` и `ithil` (ранние прототипы).

---

<a id="features"></a>
## Возможности

- **Всё с клавиатуры**: навигация в духе vim (`gg/G`, режим ввода) и курсор
  по сообщениям (`j/k`), который переходит от пузыря к пузырю и указывает, к
  какому сообщению относятся контекстное меню и действия. Мышь необязательна:
  можно щёлкнуть по чату или панели и прокручивать колёсиком.
- **Уведомления без открытого окна**: `tele daemon install` держит `tele`
  запущенным с момента входа в систему (launchd, пользовательский юнит systemd
  или Планировщик заданий), так что аккаунт остаётся на связи, а системные
  уведомления продолжают приходить, когда окно закрыто. Если открыть `tele`
  снова, он подключится к уже работающему, а не будет соединяться второй раз;
  если открыть его в двух терминалах, оба покажут один и тот же аккаунт:
  [docs/ru/configuration.md](docs/ru/configuration.md#running-without-a-window)
- **Полная поддержка Telegram**: личные чаты, группы, каналы, темы форумов,
  ответы, реакции, правки, пересылка и черновики для каждого чата,
  синхронизированные с Telegram.
- **Фото прямо в чате**: в полном качестве через протокол графики Kitty в
  kitty, Ghostty и iTerm2 3.7.0+, везде остальном блочной ANSI-графикой. `o`
  открывает изображение во встроенном просмотрщике.
- **Голосовые сообщения**: волна с длительностью и воспроизведение в приложении
  (`p`) с движущейся позицией. Внешний плеер и cgo не нужны.
- **Видео, кружки и GIF**: миниатюры прямо в чате, а выбранная GIF
  проигрывается по кругу на своём месте.
- **Отправка медиа**: `u` прикрепляет файл; нажмите ещё раз, чтобы добавить
  ещё, и они уйдут одним альбомом.
- **Прокси, как в официальных клиентах**: прокси MTProto (сервер, порт и
  секрет из ссылки `tg://proxy`, включая fake-TLS) или SOCKS5, заданный в
  конфиге только для `tele`, а не для всей оболочки, и используемый каждым
  соединением, в том числе загрузками:
  [docs/ru/configuration.md](docs/ru/configuration.md#proxy)
- **Сделан для терминала**: написан под работу в терминале с самого начала, а
  не перенесён из графического клиента. Один статический бинарник на Go,
  быстрый запуск, мало памяти.
- **Простая настройка**: один файл YAML с разумными значениями по умолчанию и
  окно настроек на `,`, которое правит тот же файл, сохраняя комментарии.

> **Запись не предусмотрена.** `tele` может _отправить_ заранее записанный
> аудио- или видеофайл, но не умеет **записывать** голосовые или кружки и не
> поддерживает голосовые и видеозвонки: это терминальный клиент без cgo, и у
> него нет средств для захвата микрофона или камеры. Это архитектурная граница,
> и в планах такой функции нет.

Подробно обо всех типах медиа, воспроизведении в приложении и необязательной
зависимости `ffmpeg`: [docs/ru/media.md](docs/ru/media.md)

---

<a id="installation"></a>
## Установка

<a id="any-unix-macos--linux--freebsd--openbsd--netbsd---one-liner"></a>
### Любой Unix (macOS · Linux · FreeBSD · OpenBSD · NetBSD): одной командой

```sh
curl -sL https://raw.githubusercontent.com/sorokin-vladimir/tele/main/scripts/install.sh | sh
```

Скрипт определяет ОС и архитектуру и ставит подходящий бинарник из последнего
релиза. Варианты:

```sh
# Последняя бета (пререлиз), ставится как `tele-beta` и уживается со стабильной:
curl -sL https://raw.githubusercontent.com/sorokin-vladimir/tele/main/scripts/install.sh | sh -s -- --beta

# Конкретная версия или свой каталог установки:
curl -sL .../install.sh | sh -s -- --version v1.9.0
curl -sL .../install.sh | PREFIX="$HOME/.local/bin" sh
```

На FreeBSD системные уведомления идут через терминал (только в терминалах,
которые это поддерживают), а для воспроизведения звука нужен работающий сервер
PulseAudio/PipeWire. Без них соответствующая функция отключается без ошибок.

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

Обе формулы записаны полностью намеренно. Если tap подключён, простой
`brew install tele` выбирает формулу из tap и сообщает об этом только строкой
`Warning: tele shadows homebrew/core/tele`, которую легко пропустить. Формулы
отличаются в одном, о чём стоит знать; это описано в разделе
[Ключ приложения](#app-key).

<a id="macos--linux---homebrew-beta-channel"></a>
### macOS / Linux: Homebrew (канал бет)

Чтобы получать последние влитые изменения раньше еженедельного стабильного
релиза, поставьте пакет беты из того же tap. Это отдельный бинарник
`tele-beta` со своими конфигом и состоянием (`~/.config/tele-beta`), поэтому он
уживается со стабильной версией:

```sh
brew tap sorokin-vladimir/tap
brew trust sorokin-vladimir/tap
brew install tele-beta
brew upgrade tele-beta   # подтягивать новые беты по мере выхода
```

Беты собираются из тегов пререлизов (`vX.Y.Z-beta.N`) и публикуются на GitHub
как пререлизы, поэтому никогда не становятся «последним» релизом. Запускается
командой `tele-beta`.

<a id="linux---binary"></a>
### Linux: бинарник

```sh
curl -sL https://github.com/sorokin-vladimir/tele/releases/latest/download/tele-linux-amd64 \
  -o ~/.local/bin/tele && chmod +x ~/.local/bin/tele
```

Для arm64 замените `amd64` на `arm64`.

<a id="debian--ubuntu--mint---apt"></a>
### Debian / Ubuntu / Mint: apt

```sh
echo 'deb [trusted=yes] https://apt.fury.io/sorokin-vladimir/ /' \
  | sudo tee /etc/apt/sources.list.d/tele.list
sudo apt update && sudo apt install tele
```

<a id="fedora--rhel--opensuse---dnf--zypper"></a>
### Fedora / RHEL / openSUSE: dnf / zypper

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

<a id="alpine---apk"></a>
### Alpine: apk

```sh
echo 'https://alpine.fury.io/sorokin-vladimir/' | sudo tee -a /etc/apk/repositories
sudo apk add --allow-untrusted tele
```

> Подписанные файлы `.deb` и `.rpm` также приложены к каждому
> [релизу](https://github.com/sorokin-vladimir/tele/releases/latest).

> Пакеты для AUR и Snap запланированы, но пока не опубликованы.

<a id="windows---binary"></a>
### Windows: бинарник

Скачайте исполняемый файл через PowerShell (от имени обычного пользователя):

```powershell
$dir = "$Env:LOCALAPPDATA\Programs\tele"
New-Item -ItemType Directory -Force -Path $dir | Out-Null
Invoke-WebRequest `
  -Uri "https://github.com/sorokin-vladimir/tele/releases/latest/download/tele-windows-amd64.exe" `
  -OutFile "$dir\tele.exe"
```

Для arm64 замените `amd64` на `arm64`. Добавьте `$dir` в `PATH` (или запускайте
`tele.exe` по полному пути) и запускайте из любого терминала. Фото там рисуются
блочной ANSI-графикой: для изображений прямо в чате нужен терминал, который
рисует изображения Kitty через символы-заполнители Unicode, а такого терминала
для Windows пока нет. [WezTerm](https://wezterm.org) поддерживает протокол
графики Kitty, но не символы-заполнители
([wezterm#7924](https://github.com/wezterm/wezterm/pull/7924)). Когда они
появятся в каком-то терминале, `photos.mode: kitty` включит изображения в чате
раньше, чем автоопределение научится его распознавать.

> Архив `.zip` с `tele.exe` (`tele_windows_amd64.zip`) приложен к каждому
> [релизу](https://github.com/sorokin-vladimir/tele/releases/latest).

> Через пакетный менеджер: [Scoop](https://scoop.sh) или [winget](https://learn.microsoft.com/windows/package-manager/):
>
> ```powershell
> scoop bucket add tele https://github.com/sorokin-vladimir/scoop-tele
> scoop install tele
> # или
> winget install sorokin-vladimir.tele
> ```

<a id="nix--nixos"></a>
### Nix / NixOS

У `tele` есть flake. Попробовать без установки:

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

В репозитории есть и оболочка `nix develop` для локальной разработки (go,
golangci-lint, lefthook).

---

<a id="first-launch"></a>
## Первый запуск

```sh
tele
```

При первом запуске `tele` создаёт:

```text
~/.config/tele/config.yml
```

Затем спрашивает:

- номер телефона
- код из SMS
- пароль двухэтапной проверки, если он есть

Когда вы вошли в аккаунт, `tele` один раз спрашивает, продолжать ли работать в
фоне как сервис. Ответьте `y`, и аккаунт останется на связи после закрытия
окна, а уведомления будут приходить и дальше; ответьте `n` или нажмите Esc, и
`tele` будет работать, только пока открыто окно, как раньше. Передумать можно
позже командами `tele daemon install` или `tele daemon uninstall`. Вопрос не
задаётся там, где сервис установить нельзя, и если он уже установлен.

<a id="upgrading-from-111"></a>
### Переход с 1.11

- Если открыть `tele`, пока работает другой, ошибки больше не будет: второй
  подключится к первому как ещё одно окно того же аккаунта.
- После входа в аккаунт `tele` один раз спрашивает, работать ли как сервис (см.
  выше).
- Флаги теперь пишутся с двумя дефисами (`--config`, `--debug`); короткий `-e`
  по-прежнему работает. Версия и инструменты тем стали командами:
  `tele version`, `tele theme check`, `tele theme dump`.
- 1.11 и 2.0 не могут одновременно работать с одним аккаунтом: завершите 1.11,
  прежде чем запускать 2.0.

Все изменения (на английском): [CHANGELOG.md](CHANGELOG.md)

<a id="app-key"></a>
### Ключ приложения

`tele` обращается к Telegram с ключом приложения: это пара `api_id` и
`api_hash`, которая указывает на приложение, а не на вас. Делать с ним ничего не
нужно: ключ есть в каждой сборке, и вход, описанный выше, работает как есть.

Сборки из опубликованных исходников (homebrew-core, Nix flake, порт для BSD,
ваш собственный `go build`) используют один общий ключ. Telegram может
отказать ключу, которым, по его мнению, пользуются слишком многие, и тогда
`tele` пишет `app key blocked by Telegram`, а не утверждает, что вы вышли из
аккаунта. Обойти это можно двумя способами, выберите тот, что проще.

Зарегистрируйте свой ключ на [my.telegram.org](https://my.telegram.org) и
пропишите его в `~/.config/tele/config.yml`. Ваш ключ всегда главнее
встроенного:

```yaml
telegram:
  api_id: 123456
  api_hash: "your api hash"
```

Задайте оба значения или ни одного: если в конфиге указано только одно, `tele`
не запустится.

Или поставьте официальную сборку, у которой собственный ключ: tap для Homebrew
выше, установочный скрипт, apt, dnf, apk, Snap, Scoop и winget ставят
бинарники, собранные релизным конвейером `tele`.

Подробнее, в том числе какая формула Homebrew с каким ключом:
[docs/ru/app-key.md](docs/ru/app-key.md)

---

<a id="commands"></a>
## Команды

| Команда                   | Что делает                                                                                      |
| ------------------------- | ----------------------------------------------------------------------------------------------- |
| `tele`                    | Открыть окно с аккаунтом                                                                        |
| `tele version`            | Напечатать версию                                                                               |
| `tele theme check [NAME]` | Показать, во что развернулся каждый слот и откуда взялись его токены, или то же для одной темы по имени |
| `tele theme dump [NAME]`  | Вывести тему слота или тему по имени как полный файл темы (`tele theme dump light`)             |
| `tele daemon`             | Держать аккаунт без окна, сохраняя соединение и уведомления; окна подключаются к нему           |
| `tele daemon status`      | Показать, как дела у `tele`, который держит аккаунт (`--json` для скриптов); код 3, если такого нет |
| `tele daemon install`     | Запускать `tele` при каждом входе в систему как сервис: launchd, юнит systemd или Планировщик заданий |
| `tele daemon uninstall`   | Остановить сервис и удалить его; состояние и конфиг не трогаются                                |

| Флаг                    | Что делает                                                                                     |
| ----------------------- | ---------------------------------------------------------------------------------------------- |
| `-c`, `--config <path>` | Путь к файлу конфигурации (по умолчанию `~/.config/tele/config.yml`)                           |
| `-e`, `--debug`         | Включить отладочный лог                                                                        |
| `--trace`               | Писать в лог чувствительные данные (id собеседников, длины сообщений). Никогда не включайте на общей или синхронизируемой файловой системе |

Все команды и флаги полностью: [docs/ru/commands.md](docs/ru/commands.md). Коды
завершения, вывод JSON и правила, по которым названы команды:
[docs/ru/cli.md](docs/ru/cli.md). Работа без окна:
[docs/ru/configuration.md](docs/ru/configuration.md#running-without-a-window).

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

Всё хранится в одном файле YAML, `~/.config/tele/config.yml`, который создаётся
при первом запуске.

Клавиша `,` открывает окно настроек: в нём все настройки `tele` в том же порядке
и с той же группировкой, что и в файле, с текущими значениями и тем, когда
изменение вступит в силу. Изменения записываются прямо в `config.yml`, с
сохранением ваших комментариев и пустых строк, так что нет разницы, править ли
файл руками или в окне настроек.

Настройки, которые меняют чаще всего:

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

Все остальные ключи и что делает каждый:
[docs/ru/configuration.md](docs/ru/configuration.md)

<a id="themes"></a>
### Темы

`tele` держит две темы одновременно и переключается между ними вслед за фоном
терминала: `ui.theme.dark` и `ui.theme.light`. Если `ui.theme` не задан,
используются встроенные `tele-dark` и `tele-light`.

Внутри бинарника поставляются восемь готовых тем: Catppuccin Macchiato,
Dracula, Gruvbox Dark, Nord, Tokyo Night (Night, Moon и Day) и Seoul256 Light.
Ставить и копировать ничего не нужно: достаточно назвать тему.

```yaml
ui:
  theme: tokyonight-night
```

Ваши собственные темы лежат файлами в `~/.config/tele/themes/`, и тема задаёт
только те токены, которые ей важны, а остальные наследует. Исходники
поставляемых тем с подробными комментариями о палитре, из которой каждая
перенесена, лежат в [`themes/`](themes/).

Как написать тему, все токены и проверки контраста:
[docs/ru/themes.md](docs/ru/themes.md)

<a id="customizing-keybindings"></a>
### Как переназначить клавиши

Клавиши по умолчанию переопределяются в разделе `keybindings:` файла
конфигурации. В сгенерированном конфиге уже перечислены все действия с их
текущими клавишами по умолчанию, закомментированные и сгруппированные по
контексту.

Синтаксис клавиш, аккорды, как разрешаются конфликты и полный список имён
действий: [docs/ru/keybindings.md](docs/ru/keybindings.md#customizing-keybindings)

---

<a id="roadmap"></a>
## Планы

Запланированная работа ведётся на открытой [**доске проекта**](https://github.com/users/sorokin-vladimir/projects/2)
и сгруппирована по [вехам](https://github.com/sorokin-vladimir/tele/milestones). Вехи соответствуют
минорным версиям (`v1.11`, `v2.0`, …); патч-релизы выходят внутри версии по мере исправлений.

| Релиз               | Чему посвящён                                                                                                                                                                                                                            |
| ------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `v1.11` _(готово)_  | Настройки и люди: окно настроек в приложении, окно профиля, аватары, темы внутри бинарника, а также оставшиеся пробелы в распространении и аудите                                                                                          |
| `v2.0` _(готово)_   | Один владелец соединения с Telegram: окна, которые подключаются к одному `tele`, сервис, который поддерживает его работу с момента входа в систему, уведомления без открытого окна, один бюджет на обращения к Telegram                    |
| `v2.1`              | Скрипты: команды, которые выводят список чатов и тем, читают чат и отправляют в него, с выводом в JSON; затем форматированные сообщения                                                                                                   |
| `Backlog`           | Для опытных пользователей и платформы: поиск (полнотекстовый по истории, палитра команд, поиск внутри чата), голосовые и кружки, отложенная отправка, команды ботов и встроенные клавиатуры, секретные чаты, опросы, расширенные движения vim, переход по уведомлению |

Работа также разбита по направлениям (Security & Reliability, Architecture & Performance,
Feature Completeness, Power User & Polish) в поле **Theme** на доске.

---

<a id="build-from-source"></a>
## Сборка из исходников

Нужен Go 1.26+. Получившийся бинарник содержит общий ключ приложения, описанный
в разделе [Ключ приложения](#app-key), поэтому работает без регистрации:

```sh
git clone https://github.com/sorokin-vladimir/tele
cd tele
go build -o tele ./cmd/tele/
```

Сборка со своим ключом приложения, оболочка `nix develop` и `vendorHash` во
flake (на английском): [docs/development.md](docs/development.md)

---

<a id="documentation"></a>
## Документация

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

GPL-3.0: можно свободно использовать и форкать; производные работы должны
оставаться открытыми.

---

Сделано с помощью:

- [gotd/td](https://github.com/gotd/td)
- [bubbletea](https://github.com/charmbracelet/bubbletea)
- [lipgloss](https://github.com/charmbracelet/lipgloss)
- вдохновлено [lazygit](https://github.com/jesseduffield/lazygit)
