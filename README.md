# ytmgo

YouTube Music 桌面客户端，当前开发版本 **0.5.0**。主界面用 **MyGo 0.7.1 原生 UI** 绘制，曲库由 Go 访问，媒体由按需 YouTube.js 引擎与有界流式传输处理，音频由 MyGo 输出到系统音频设备。保留侧栏、搜索、歌曲与专辑列表、底部播放条、正在播放面板、队列、歌词、沉浸模式、封面/视频切换，以及四语言和深浅色设计。

音乐界面与播放控件使用 MyGo 原生 UI。登录时单独启动 Wry 系统 WebView：macOS 使用 WKWebView，Windows 使用 WebView2，Linux 使用 WebKitGTK。登录会话通过私有父子进程管道交接后，登录子进程退出；主界面与播放不依赖这个窗口。没有浏览器扩展、HTTP 登录桥或 OAuth 配置。外部链接仍使用系统默认浏览器。

账号状态由服务端明确返回的 `logged_in` 和 `has_unlimited_entitlement` 决定，每五分钟及应用回到前台时刷新。Premium 模式不加载 jsdom 或播放证明模块；普通账号和未登录模式在需要播放时启动证明引擎。切换时退出并回收旧引擎，同账号的会员变化保留正在播放的音频，换账号或退出则撤销旧账号的媒体流。请求失败或字段缺失不会被当成会员过期。

账号、整曲播放、内存与安装包测试记录在 [VERIFICATION.md](VERIFICATION.md)。Premium 原生播放实测整应用中位数约 359 MiB，不能把约 74 MiB 的 Node 辅助进程当成整应用占用。匿名、普通账号和 Premium 的代码与实际验证范围分别记录；新会话存储的恢复与清理验证也单独记录。设计约定见 [DESIGN.md](DESIGN.md)。

## 运行和构建

当前完整媒体包交付目标仍为 **macOS 27 / Apple Silicon**，需要 Go **1.27.3+**。构建登录组件还需要 Rust/Cargo；macOS 构建使用 Xcode Command Line Tools，Linux 使用 GTK 3 与 WebKitGTK 4.1 开发包。安装后的应用无需 Rust、Python、Homebrew 或终端 PATH。Windows 登录要求 WebView2 Runtime ≥101.0.1210.39；Linux 登录要求 WebKitGTK 4.1，账号保存需要系统 Secret Service。跨平台登录构建与整个媒体包的发行验收分别记录。

```sh
make check
make build
open build/darwin-arm64/ytmgo.app
```

`make build` 会打包锁定的 YouTube.js 18.3.0、Node 26.12.1、完整标准证明客户端及精简 FFmpeg 9.0.2。播放源代码及新包中不包含 yt-dlp；没有客户端切换或旧播放器 fallback。版本、校验值、许可证、上游源码和构建信息随包保存。证明客户端采用 GPL-3.0，依赖许可材料的剩余缺口已单独记录，尚未做公开分发验收。详细说明见 [scripts/runtime/README.md](scripts/runtime/README.md)。

本地默认产物为 `build/darwin-arm64/ytmgo.app` 和 `build/darwin-arm64/ytmgo-0.5.0-arm64.dmg`，采用 ad-hoc App 签名和未签名 DMG。GitHub Actions 已提供手动触发的测试打包、Developer ID 签名及 Apple 公证模式；配置证书、私钥和使用方法见 [macOS Actions 配置指南](docs/MACOS_ACTIONS.md)。选择 `notarized` 并勾选 `publish_release` 后，工作流会发布可下载的 GitHub Release 和签名更新 feed。重新构建导致 ad-hoc 身份变化时，macOS 可能要求重新授权访问本应用保存在钥匙串中的加密密钥。其他操作系统未作为本轮交付目标。

正式安装包见 [GitHub Releases](https://github.com/mreasonyang/ytmgo/releases/latest)。0.5.0 已通过签名、公证和真实公开包升级检查，范围与产物身份见 [自动升级验收](docs/AUTO_UPDATE_VERIFICATION.md)。

开发运行：

```sh
make dev
```

## 使用

- 首页、探索、搜索和专辑/歌单页面直接访问 YouTube Music。首页支持继续加载，前进/后退保留浏览位置所对应的页面数据。
- 点击歌曲开始播放；支持播放/暂停、上一首/下一首、seek、音量、静音、随机与单曲/列表循环。右键曲目可加入队列。
- 点击底部封面打开正在播放，可查看队列或歌词。视频由独立原生解码器按需显示，最高 480p / 24fps，按音频时钟同步；隐藏窗口、关闭播放视图或切回封面会释放视频解码器。
- 宽窗口使用完整侧栏和停靠面板；中等窗口使用图标侧栏与浮动面板；紧凑窗口使用底部导航和迷你播放器。
- 设置支持 English、简体中文、繁體中文、日本語，以及深色、浅色、跟随系统。修改语言或外观会即时重绘，不重建窗口或播放器。
- 原生菜单、托盘、全局播放快捷键和系统 Now Playing 共用同一播放状态。
- 安装到可写目录后自动检查更新（启动后最早 10 秒，之后每日一次），也可使用 **ytmgo → 检查更新**。原生更新窗口支持立即安装、稍后、跳过，以及可选的自动下载；下载完成后重启安装。0.4.0 没有更新器，需要先手动安装一次 0.5.0。

## 账号与数据

点击登录会打开 Google 官方登录页面；在窗口里选择账号并完成 Google 要求的验证。正常登录不设应用自定义的强制到期时间；登录成功、关闭窗口或退出应用时结束登录子进程。程序读取当前所选账号的会话，再向 YouTube 验证账号与会员状态后保存。关闭窗口取消本次登录，原有账号保留。会话只从系统 WebView 的私有存储读取，通过匿名管道交接，不经过浏览器扩展、本地 HTTP 服务或临时 Cookie 文件。

macOS 登录窗口目前不支持 Google passkey。Apple 要求 WKWebView 的 passkey 服务与应用建立关联域关系，ytmgo 无法自行取得 Google 的关联授权；请使用 Google 页面提供的其他验证方式。没有其他验证方式的账号目前无法完成登录。Windows 的 WebView2 支持 FIDO2，但本应用与具体 Google passkey 提供者的组合尚未实测；Linux 使用 WebKitGTK 4.1 ≥2.50.2，避免旧版本在带 WebAuthn 第二因子的 Google 账号上卡住，这不代表 Linux 已支持 passkey。[Apple 说明](https://developer.apple.com/documentation/authenticationservices/supporting-passkeys)、[WebKitGTK 修复说明](https://webkitgtk.org/2025/11/19/webkitgtk2.50.2-released.html)。

完整会话使用 AES-256-GCM 加密保存在应用数据目录的 `account/session.enc`；系统安全存储只保管一个 32 字节密钥，macOS 使用 Keychain。文件按原子替换保存；macOS/Linux 目录权限为 0700、文件为 0600，Windows 使用当前用户的数据目录和 Credential Manager。重启后先重新核验服务端状态，再启用对应播放模式。没有明文凭据文件、旧分片读取、旧 WebKit 数据迁移或存储 fallback；使用过分片版本的用户需要重新连接一次账号。确认退出会停止播放、清空本应用的个人数据，并删除加密会话文件与密钥；外部浏览器的 Google 登录状态不受影响。

遇到 Google 验证、登录或同意页面跳转时，应用直接说明服务端要求及当前请求无法完成；未知跳转也会如实说明。曲库和播放保留普通重试，不提供未经证实的网页验证步骤。已连接账号会保留，不会把请求限制认定为会员过期。普通 403、429 或网络错误不会被称为验证码，服务端返回的跳转地址不会被保存或打开。

播放器创建时只启动轻量控制/媒体桥接进程，不预热 SDK 或 jsdom。当前曲目播放时最多准备下一首的媒体和首批 PCM；随机点播与初次启动仍需网络解析。快速切歌、账号切换、队列变化及 seek 会取消过期准备。

播放器脚本分析在临时 bootstrap 子进程中完成，回收后由一个受限引擎恢复有界缓存。Premium 引擎仅加载 SDK 与签名处理；普通/未登录引擎按需加载标准证明客户端和 jsdom。会员变化会销毁旧引擎并在需要解析时创建新引擎，不让两种模式长期同时驻留。凭据与签名媒体地址只走私有内存管道，不进入 argv、环境、日志或证据文件。

当前实现没有官方广告素材序列和真实广告进度上报；不能把公开歌曲播放成功描述成广告流程验收通过，也不使用推算等待时间模拟广告。

远端媒体地址只留在 Node 内存中；Go 与 FFmpeg 仅连接有界回环流式通道，私有地址通过 stdin/stdout 管道传递，不进入命令行或日志。远端按 64 KiB 区间读取、本地按 32 KiB 背压转发，最多四个媒体能力和四条上游连接，覆盖音频、下一首、按需视频及切换期间的资源。停止对应媒体会取消该流并回收解码器；退出应用或终止会话会回收所属 runtime 子进程，不保留整首媒体缓存。

本地 `native-diagnostics/audio-status.json` 与 `video-status.json` 仅保存启动状态或最近一次失败的固定类别、时间和应用版本；播放初始化失败还可包含封闭的阶段、HTTP 状态码、跳转类别与内容类型。不会保存媒体地址、请求头、账号数据或原始日志。

## 代码与测试

```text
updates.go                  原生更新窗口、菜单与维护命令
main.go / controller.go      原生窗口、菜单、导航、队列与请求生命周期
native_views.go              原生屏幕、响应式布局和交互
native_style.go              原有色板与矢量图标
account.go                  登录会话交接、权益轮询和退出
internal/accountsession/    会话校验与加密存储
internal/accountlogin/      登录子进程与有界私有协议
native/account-login/       三平台 Wry 临时登录窗口
scripts/login/              登录组件构建与平台校验
artwork.go                  有界图片下载与解码缓存
video_controller.go         视频视图可见性及帧更新
internal/catalog/           类型化曲库、分页、队列、歌词和账号接口
internal/player/            独立解析、有界音视频解码、系统音频输出
locales/                    四语言目录
scripts/runtime/            可重现的播放器组件打包
scripts/diagnostics/        不读取 argv/env 的内存测量
```

```sh
make check
go test -race ./...
YTMGO_RUNTIME_DIR="$PWD/build/runtime" go test ./internal/player
YTMGO_LIVE_CATALOG=1 go test -v -run TestLive ./internal/catalog
YTMGO_RUNTIME_DIR="$PWD/build/runtime" YTMGO_LIVE_PLAYBACK=1 go test -v -run TestNativePlaybackLive ./internal/player
YTMGO_RUNTIME_DIR="$PWD/build/runtime" YTMGO_LIVE_PREPARED=1 go test -v -run TestNativePreparedPlaybackLive ./internal/player
YTMGO_UI_SNAPSHOT_DIR="$PWD/build/native-screenshots" go test -run '^TestNative' .
```

自动布局截图使用 MyGo 软件渲染器，属于原生视图测试；真实应用、音频设备、网络和内存证据分别记录，互不替代。历史 0.3 审计保留在 `docs/audits/`，只作为设计参考和历史证据。
