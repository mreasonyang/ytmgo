# ytmgo

YouTube Music 桌面客户端，当前版本基线 **0.6.0**。主界面用 **MyGo 0.7.1 原生 UI** 绘制，曲库由 Go 访问，媒体请求、格式选择和有界流式传输由 Go 处理，音频由 MyGo 输出到系统音频设备。保留侧栏、搜索、歌曲与专辑列表、底部播放条、正在播放面板、队列、歌词、沉浸模式、封面/视频切换，以及四语言和深浅色设计。

音乐界面与播放控件使用 MyGo 原生 UI。登录时单独启动 Wry 系统 WebView：macOS 使用 WKWebView，Windows 使用 WebView2，Linux 使用 WebKitGTK。登录会话通过私有父子进程管道交接后，登录子进程退出；主界面与播放不依赖这个窗口。没有浏览器扩展、HTTP 登录桥或 OAuth 配置。外部链接仍使用系统默认浏览器。

账号状态由服务端明确返回的 `logged_in` 和 `has_unlimited_entitlement` 决定，每五分钟及应用回到前台时刷新。Premium 模式不创建 DOM 或播放证明环境；普通账号和未登录模式在需要播放时启动证明引擎。切换时退出并回收旧引擎，同账号的会员变化保留正在播放的音频，换账号或退出则撤销旧账号的媒体流。请求失败或字段缺失不会被当成会员过期。

当前分支已迁移到无 Node/jsdom 的原生运行时，并完成 macOS 匿名整曲播放、自然切歌、暂停恢复及整应用内存实测；普通账号、Premium、Windows 原生执行和长时耐久验收仍待完成。稳定播放时整应用约 357–361 MiB，尚不能宣称内存优化完成。测试范围与证据见 [VERIFICATION.md](VERIFICATION.md)，旧版 Node 数据不作为新实现的内存结果。设计约定见 [DESIGN.md](DESIGN.md)。

## 运行和构建

运行时构建覆盖 macOS、Windows、Linux，需要 Go **1.27.3+**、Rust/Cargo、Python 3 和 C 编译工具链。macOS 使用 Xcode Command Line Tools；Windows 的 FFmpeg 使用 MSYS2 UCRT64；Linux 使用 GTK 3 与 WebKitGTK 4.1 开发包。安装后的应用无需 Node、npm、Rust 或 Python。Windows 需要 WebView2 Runtime，Linux 需要 WebKitGTK 4.1 与 Secret Service。三平台 CI 构建和完整发行验收分别记录，不能以交叉编译代替原生运行验证。

```sh
make check
make build
open build/darwin-arm64/ytmgo.app
```

`make build` 打包 Go 主程序、小型原生脚本宿主和精简 FFmpeg 9.0.2。动态签名使用 QuickJS；普通账号和未登录证明使用独立、隐藏、非持久化的系统 WebView。只嵌入 YouTube.js 的脚本提取器、Meriyah 与 MIT BgUtils 的必要代码，不分发整个 SDK、Node、jsdom 或 npm 依赖目录。构建使用锁定源码归档与 Go 版 esbuild。许可证和构建信息随包保存；FFmpeg 仍使用 LGPL。详细说明见 [scripts/runtime/README.md](scripts/runtime/README.md)。

本地默认产物为 `build/darwin-arm64/ytmgo.app` 和 `build/darwin-arm64/ytmgo-0.6.0-arm64.dmg`，采用 ad-hoc App 签名和未签名 DMG。GitHub Actions 已提供手动触发的测试打包、Developer ID 签名及 Apple 公证模式；配置证书、私钥和使用方法见 [macOS Actions 配置指南](docs/MACOS_ACTIONS.md)。选择 `notarized` 并勾选 `publish_release` 后，工作流会发布可下载的 GitHub Release 和签名更新 feed。重新构建导致 ad-hoc 身份变化时，macOS 可能要求重新授权访问本应用保存在钥匙串中的加密密钥。跨平台运行时的实际验收状态以验证记录为准。

当前版本沿 `0.6.X` 迭代，末位为 0–999 的整数，不补前导零：`0.6.0`、`0.6.1`、…、`0.6.999`。仅正式发版时递增末位；普通修复、提交与本地构建不自动升版本，末位未耗尽不因日常迭代提升次版本。发布配置和更新包校验均限制末位范围。

正式安装包见 [GitHub Releases](https://github.com/mreasonyang/ytmgo/releases/latest)。0.5.1 已通过签名、公证和从公开 0.5.0 包升级的检查，范围与产物身份见 [自动升级验收](docs/AUTO_UPDATE_VERIFICATION.md)。

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
- 原生菜单、托盘、全局播放快捷键和系统 Now Playing 共用同一播放状态。全局快捷键可在设置中直接录入或停用；应用内按空格即可播放或暂停。
- 安装到可写目录后自动检查更新（启动后最早 10 秒，之后每日一次），也可使用 **ytmgo → 检查更新**。原生更新窗口支持立即安装、稍后、跳过，以及可选的自动下载；下载完成后重启安装。0.4.0 没有更新器，需要先手动安装一次 0.5.0。

## 账号与数据

点击登录会打开 Google 官方登录页面；在窗口里选择账号并完成 Google 要求的验证。正常登录不设应用自定义的强制到期时间；登录成功、关闭窗口或退出应用时结束登录子进程。程序读取当前所选账号的会话，再向 YouTube 验证账号与会员状态后保存。关闭窗口取消本次登录，原有账号保留。会话只从系统 WebView 的私有存储读取，通过匿名管道交接，不经过浏览器扩展、本地 HTTP 服务或临时 Cookie 文件。

macOS 登录窗口目前不支持 Google passkey。Apple 要求 WKWebView 的 passkey 服务与应用建立关联域关系，ytmgo 无法自行取得 Google 的关联授权；请使用 Google 页面提供的其他验证方式。没有其他验证方式的账号目前无法完成登录。Windows 的 WebView2 支持 FIDO2，但本应用与具体 Google passkey 提供者的组合尚未实测；Linux 使用 WebKitGTK 4.1 ≥2.50.2，避免旧版本在带 WebAuthn 第二因子的 Google 账号上卡住，这不代表 Linux 已支持 passkey。[Apple 说明](https://developer.apple.com/documentation/authenticationservices/supporting-passkeys)、[WebKitGTK 修复说明](https://webkitgtk.org/2025/11/19/webkitgtk2.50.2-released.html)。

完整会话（Cookies 与账号选择信息）使用 AES-256-GCM 加密保存在应用数据目录的 `account/session.enc`。系统安全存储只保管一把 32 字节随机密钥，固定条目名为 `account-session-key`；macOS 使用 Keychain。打开登录窗口前先准备密钥：复用当前进程已恢复的密钥，或读取固定条目；仅在明确不存在时创建。授权被拒绝、存储锁定或密钥损坏时，直接在登录前报错。登录与二次验证完成后，使用内存中已准备的密钥和新的随机 nonce 加密，原子覆盖会话文件，不再访问系统安全存储。准备密钥不会改动已保存的会话；登录取消或文件保存失败时保留原文件和密钥，首次准备创建的密钥也可供重试复用。退出登录同时清除内存中的密钥。只读写一种会话格式，没有旧格式读取、迁移、随机密钥编号或明文存储备用路径。

macOS 自动恢复不弹出钥匙串授权；用户主动点击“恢复账号”或重新登录时允许系统请求授权。无法恢复的登录数据可以在设置中明确清除，再重新登录。macOS/Linux 目录权限为 0700、文件为 0600，Windows 使用当前用户的数据目录和 Credential Manager。重启后先重新核验服务端状态，再启用对应播放模式。确认退出会停止播放、清空本应用的个人数据，先删除加密会话文件，再删除唯一密钥；密钥删除失败可以重试，不会因文件已不存在而漏删。外部浏览器的 Google 登录状态不受影响。

遇到 Google 验证、登录或同意页面跳转时，应用直接说明服务端要求及当前请求无法完成；未知跳转也会如实说明。曲库和播放保留普通重试，不提供未经证实的网页验证步骤。已连接账号会保留，不会把请求限制认定为会员过期。普通 403、429 或网络错误不会被称为验证码，服务端返回的跳转地址不会被保存或打开。

播放器创建时不启动脚本宿主，也不预热证明环境。当前曲目播放时最多准备下一首的媒体和首批 PCM；随机点播与初次启动仍需网络解析。快速切歌、账号切换、队列变化及 seek 会取消过期准备。

播放器脚本分析在临时 QuickJS 进程中完成，随后立即回收；Go 仅保留有界提取代码。签名执行每次使用新 JS 环境。Premium 只在需要动态签名时启动解释器，普通/未登录模式按需创建独立的隐藏证明环境。会员变化或到期会销毁旧证明环境。账号凭据只留在 Go 中，不发送到脚本宿主；签名媒体地址不进入 argv、环境、日志或证据文件。

当前实现没有官方广告素材序列和真实广告进度上报；不能把公开歌曲播放成功描述成广告流程验收通过，也不使用推算等待时间模拟广告。

远端媒体地址只留在 Go 媒体租约中；FFmpeg 只连接每个解码器的随机回环能力地址，通过 stdin 接收。Go 直接按音频 64 KiB、视频 512 KiB 区间请求远端，以 32 KiB 背压转发；最多四个租约与四条活动请求，不缓存整曲。账号身份变化撤销旧租约；同账号会员变化保留当前媒体，取消过期预加载。

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
