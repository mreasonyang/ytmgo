# ytmgo

使用 [MyGo](https://github.com/egoist/mygo) 开发的 YouTube Music 桌面客户端。当前版本 **0.3.0**：从零重写的界面——侧栏与歌单、可停靠的正在播放面板、沉浸模式、紧凑窗口的底部标签栏与迷你播放器；深色 / 浅色 / 跟随系统外观；界面支持 English、简体中文、繁體中文、日本語。设计规范见 [DESIGN.md](DESIGN.md)。

## 使用

右上角「登录」会打开独立的应用窗口，显示 Google 官方登录页。完成登录并返回音乐服务后，登录窗口关闭，主界面刷新。会话由应用内的原生 WebView 管理；不需要复制 cookie、粘贴请求头或安装 Python。

界面的导航、顶栏、播放栏、正在播放面板与设置均由 ytmgo 绘制；首页、探索、搜索、音乐库、歌单、播放队列与歌词使用 YouTube Music 的实时页面内容，并按 ytmgo 的设计系统重新排版。点一首歌即可播放，画布保持在当前页面；宽窗口首次播放时自动停靠正在播放面板（队列 / 歌词 / 相关内容），⤢ 进入沉浸模式。

界面语言默认跟随系统（zh-TW、zh-HK → 繁體中文；zh-CN → 简体中文；未匹配 → English），可在「设置 → 语言」或「视图 → 语言」中指定；切换后音乐界面重新加载，服务内容也随之切换为同一语言。

它依赖服务的网页结构，服务更新后可能需要适配（集中在 `web/shell/bridge.js`、`canvas.css` 与 `nowplaying.css` 的播放页部分）。这不是独立重写的 YouTube Music 数据 API。

## 开发与构建

需要 Go **1.27.3+**，MyGo 固定为 **v0.6.4**。Go 默认工具链自动下载可以获取所需版本；没有 Node/Bun 前端打包步骤。

```sh
go tool mygo dev
make check
make build
```

macOS Apple Silicon 产物：`build/darwin-arm64/ytmgo.app` 和 `build/darwin-arm64/ytmgo 0.3.0 arm64.dmg`。构建使用 ad-hoc 签名，未做 Apple 公证。此版本原生运行检查仅覆盖 macOS arm64，历史版本的跨平台结果不能代替本版本验收。

界面文本全部来自 `locales/*.json`（ARB / ICU MessageFormat）；修改后运行 `go tool mygo generate` 重新生成 `locales/catalog_gen.go`。`make check` 同时运行 `mygo i18n check`。

## 控制

| 操作 | 快捷键 |
| --- | --- |
| 播放 / 暂停 | ⌘/Ctrl + P；全局 ⌘/Ctrl + Shift + Space |
| 上一首 / 下一首 | ⌘/Ctrl + ← / → |
| 音量 / 静音 | ⌘/Ctrl + ↑ / ↓；⌘/Ctrl + Shift + M |
| 搜索 | ⌘/Ctrl + K，或 / |
| 正在播放面板 / 沉浸模式 / 侧栏 | ⌘/Ctrl + J / ⌘/Ctrl + Shift + F / ⌘/Ctrl + B |
| 首页 / 音乐库 | ⌘/Ctrl + Shift + H / ⌘/Ctrl + L |
| 后退 / 前进 | ⌘/Ctrl + [ / ] |
| 快捷键一览 | ⌘/Ctrl + / |
| 关闭一层（弹层 → 沉浸 → 浮动面板） | Esc |

macOS 关闭主窗口后保留应用，Dock、托盘或菜单可恢复；⌘Q 彻底退出。外部链接使用 Brave，未安装时提示，不自动切换浏览器。

## 账号与数据

- 登录窗口只在允许的 Google / YouTube 官方主机之间导航；密码、验证码直接输入 Google 页面。应用代码不读取或导出它们，也不读取浏览器的账号数据。
- 主窗口和登录窗口共用系统 WebView 的持久化会话。Cookie、网页存储与缓存会保存在本机；这不是“敏感信息完全不落盘”的设计。应用没有额外实现凭据钥匙串方案。
- 未登录或登录状态尚未确定时，侧栏隐藏退出/清理操作，原生账号菜单只显示登录入口；确认官方导航栏已显示登录后的账号头像时，两处退出入口才显示。
- 登录后，侧栏底部「退出登录 / 清除账号数据」，或原生「账号 → 退出登录并清除本机账号数据…」会先确认，再停止播放、销毁音乐/登录/旧版页面，并通过运行时清除所有账号的 Cookie、local/session storage、IndexedDB、Service Worker 和缓存。成功后显示退出结果，点击「返回首页」重新浏览；首页随后可能生成新的匿名 Cookie。
- 清理保留外观、语言设置及云端音乐库，不清除其他浏览器的会话，也不撤销 Google 服务端或其他设备的会话。它不负责擦除系统诊断、系统备份或磁盘历史副本。仅退出程序（⌘Q）仍保留登录状态。
- 清理失败显示重试入口；清理前持久化不含账号数据的 `signout-pending` 标记。若中途关闭，下一次启动先继续清理，再允许打开服务页面。
- 已使用独立测试应用验证模拟会话的原生清理和重启后的消失；真实 Google 账号的登录、云端音乐库及会话恢复仍需账号验收。
- 远程音乐页面和账号页均没有受信任的 Go IPC 权限。外观切换仅允许音乐来源发起三种固定指令；账号状态只传递固定的已登录/未登录标记，不传递账号身份；退出指令还要求用户点击，且仅打开原生确认框。
- 外观与语言偏好分别保存在应用数据目录的 `appearance` 与 `language` 文件。
- 0.3.0 移除了旧版「本地收藏」窗口及其匿名曲库请求；已有的 `library.json` 不会被读取或删除。
- 播放、广告、账号验证、Premium 权益、曲目可用性与地区限制由原服务决定；不提供离线下载。隐藏迷你视频只是界面行为，不代表独立音频流或减少视频流量。

## 验证与结构

[验收记录](VERIFICATION.md) 记录各版本检查。`make check` 包含 Go 测试、Go vet、28 项界面外壳测试（无浏览器的 DOM 替身）与 `mygo i18n check`；自动测试不代表真实账号已经登录成功。

```text
main.go                 窗口生命周期、菜单（含语言菜单）、托盘
music.go                音乐主窗口、注入外壳、独立登录窗口与页面边界
i18n.go                 外壳文本载荷、语言切换与重新本地化
logout.go               退出确认、页面销毁、清理结果及重试
locales/                四种语言的消息目录与生成的 Go 访问器
web/shell/              界面外壳：设计令牌、组件与样式（顺序见 bundle.json）
internal/navigation/    音乐、登录、外部链接、外观与语言指令边界
internal/language/      界面语言、服务 hl 映射与选择存储
internal/appearance/    外观偏好
internal/session/       不读取凭据的清理流程与中断恢复标记
```

本项目是独立客户端，与 YouTube、Google 无隶属关系。音乐、图片和账号服务由原服务及权利人提供。
