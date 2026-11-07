# 媒体

> 本文译自 [media.md](../media.md)，对应提交 `a8ea9ea`。译文与英文版不一致时，以英文版为准。本译文尚未经过母语者审校。如果发现翻译错误，欢迎[提交 issue](https://github.com/sorokin-vladimir/tele/issues)。

`tele` 在聊天中直接显示什么、自己播放什么、交给外部程序处理什么。相关设置都位于 `photos:` 下，见 [configuration.md](configuration.md#photos)。

<a id="photos"></a>
## 图片

在 kitty、Ghostty 和 iTerm2 3.7.0 及以上版本中，图片通过 Kitty 图形协议以高画质直接显示在聊天中，其他终端则以 ANSI 字符块画回退显示。tmux 和 screen 不转发该协议，因此在它们中间图片显示为字符块画。

按 `o` 在应用内的查看器中打开原画质图片，边框上显示发送者和时间；按 `O` 在外部查看器中打开。

<a id="photos-on-windows"></a>
### Windows 上的图片

在 Windows 上图片显示为字符块画。聊天内图片需要终端能通过 Unicode 占位符单元格绘制 Kitty 图片，而目前还没有 Windows 终端能做到。[WezTerm](https://wezterm.org) 支持 Kitty 图形协议，但不支持占位符（[wezterm#7924](https://github.com/wezterm/wezterm/pull/7924)）。一旦有终端支持了它们，在自动检测学会识别该终端之前，`photos.mode: kitty` 就能开启聊天内图片。

<a id="voice-messages"></a>
## 语音消息

显示带时长的振幅波形，按 `p` 在应用内播放，并有移动的播放位置。

所有平台上的播放都不依赖 cgo：Opus/Ogg 用纯 Go 解码，声音在 macOS 和 Windows 上通过 `oto` 输出，在 Linux 上通过 PulseAudio/PipeWire 协议输出。在 Linux 上需要有正在运行的 PulseAudio 或 PipeWire 服务器，桌面环境默认就是如此。

<a id="video-and-round-video-кружки"></a>
## 视频和圆形视频

在聊天中显示缩略图预览，叠加 `▶` 和时长；圆形视频显示为圆形。按 `o` 用系统播放器播放。

<a id="gifs"></a>
## GIF

在聊天中显示带 `GIF` 标记的静态缩略图。在 Kitty 图形模式下，选中的 GIF 会在原位静音循环播放。这需要 `ffmpeg`，见下文。

<a id="audio-music"></a>
## 音频（音乐）

显示表演者、标题和时长。其他媒体类型显示为带标签的占位符。

<a id="sending-media"></a>
## 发送媒体

用 `u` 附加磁盘上已有的文件（图片、视频、语音、音乐、文档），发送前确认以哪种类型发送。再按一次 `u` 可以添加更多文件：它们会作为一个相册发送，图片和文档会自动分成不同的相册。

<a id="recording-is-out-of-scope"></a>
## 不支持录制

`tele` 可以*发送*预先录好的音频或视频文件，但不能在应用内**录制**语音消息或圆形视频，也不支持实时语音或视频通话。作为不依赖 cgo 的终端客户端，它没有采集麦克风或摄像头的能力。这是架构上的边界，而不是路线图上缺少的功能：录制没有计划，待办列表中也没有。

<a id="optional-dependency-ffmpeg"></a>
## 可选依赖：`ffmpeg`

在 `PATH` 中安装 `ffmpeg`（连同 `ffprobe`），即可在聊天中播放 GIF（需要解码帧），并在发送视频时附带时长、尺寸和缩略图。它完全是可选的：没有它，GIF 保持静态，视频照样可以发送，因为 Telegram 会在服务器端生成预览。
