# Spotify 原生 UI 端到端验收

日期：2026-10-29，Asia/Shanghai。结论：**PARTIAL**。

已完成本机 macOS 匿名测试，并在系统授权后补测真实 Premium 账号。浏览与基本播放路径通过；现有问题包括匿名首页分页验收失败、两处轻度 UI 问题，以及 Premium 曲尾 seek 在两首歌曲上共三次实际超时。另有一次 race 套件超时尚未稳定复现。整体不能签署全部功能 PASS。

## 被测版本与证据边界

- 仓库 `mreasonyang/ytmgo`，`main` 从 `f503800` fast-forward 到 **`9b8766e03cc76a82c5cd5342ee85ade5a7ad7665`**。产品源文件没有修改。
- macOS **27.0.1 / arm64**，MyGo **0.7.1**，应用 **0.4.0**。
- `make build` 成功。正式身份包的 executable SHA-256 为 `00917f8b2f4ad2a0d2abb4a44edeedf279d4bfdd5bff7621101526b575b8f1c8`；Go build metadata 指向本次提交、`vcs.modified=false`。App strict/deep 签名验证与 DMG 校验通过。签名是 ad-hoc；未安装到 Applications，未做公证或公开发布。
- 正式身份包完成四语言 / 主题 / 搜索 / 专辑窗口操作；已有账号恢复停在 `restoring`，系统安全代理正在运行，疑似等待新签名的钥匙串授权。工具禁止访问 SecurityAgent。用户先选择完成无需账号部分，随后完成授权并要求继续；后续结果见 Premium 追加验证。
- 后续主 UI / 播放使用**独立匿名副本**：重签名前主二进制与正式包逐字节相同，只更改 bundle 名称、标识和签名，从而使用独立数据目录；诊断脚本没有复制、直接读取或更改已有账号会话。
- 最小尺寸用另一个诊断副本：只将 `main.go` 初始窗口从 1440×940 改成 380×640，并改独立窗口状态键；生产 UI、控制器、网络和媒体运行时相同。实际截图 760×1280 pixels，Retina 2×。这是实际原生渲染验收，不等同于正式身份安装包的全部验证。修改见 [minimum-window.patch](minimum-window.patch)。
- 完整构建哈希和副本差异见 [artifact.json](artifact.json)，逐项结果见 [results.json](results.json)。

## 验证结果

| 层级 | 本次结果 |
| --- | --- |
| `make check` | PASS：Go tests、vet；66 Node offline tests；manifest / protocol 各 5；登录 Python tests 10；四语言各 198 条文案 |
| `go test -race ./...` | 首次 FAIL 一次；定向 10 次 PASS；完整复跑 PASS，详情见 R01 |
| 诊断工具测试 | PASS，24 项 |
| 原有原生视图测试 | PASS；43 张软件渲染截图，包含四语言、深浅色、响应式、账号展示和错误状态 |
| 额外断点覆盖 | PASS，360 组：10 个宽度 × 4 语言 × 3 账号展示状态 × 3 页面模式，内容高度 640；这里的账号状态是 fixture |
| 匿名在线曲库 | 搜索、探索、专辑、艺人、队列和歌词 PASS；Home continuation FAIL |
| 匿名媒体链路 | PASS：真实非零 PCM、推进的系统音频时钟、暂停、seek、预加载切歌、并发视频、关闭 |
| 完整曲目解码 | 两首均 PASS：212.547 / 369.627 秒 PCM，元数据 212.561 / 369.641 秒；EOF、decoder 和 bridge 检查通过 |
| 真实原生窗口 | 宽屏、约 960 宽半屏、约 422 宽紧凑、380×640 最小诊断窗口均实际操作 |
| 真实账号 / Premium | 追加验证：账号恢复、曲库分页、主要播放及取消管理窗口 PASS；曲尾 seek FAIL，见 F04 |

断点宽度为 380、420、760、761、900、1000、1001、1180、1181、1440。原有截图见 [snapshots/](snapshots/)，审计 probe 源码见 [spotify_audit_test.go.txt](spotify_audit_test.go.txt)。这些软件测试只证明对应断言，不替代真实窗口交互。

### 实际操作通过的路径

- 英文搜索、CJK 粘贴和 Enter 提交；⌘K 聚焦搜索。输入编辑器内的空格不会切换播放。未验证物理输入法的候选窗与 composition。
- 搜索进入专辑；Home / Explore / Library；Back / Forward 恢复之前的查询和页面内容；⌘B 折叠侧栏。
- 搜索结果播放、暂停与继续、上一首 / 下一首、队列追加与选播、队列滚动、随机队列、循环 off → all → one → off。
- 暂停位置 `118.943958333s`；鼠标 seek 至 `206.000454678s`，键盘 Left 再到 `203.90202s`；seek 完成后仍保持暂停。
- 曲尾 seek 至 `246.355034992s` 后恢复，歌曲自然结束并进入下一首，下一首位置推进至 `6.594666666s`，保持静音。
- 鼠标与键盘调整音量到 `0.14`；静音变为 0；取消静音恢复 `0.14`。
- 实际视频帧显示；方形视频在 16:9 容器内保留比例；Cover / Video 切换与关闭面板继续音频。没有复现旧版视频垂直裁切。
- 紧凑模式从底部封面进入 Now Playing，可访问队列和真实歌词。380×640 下播放面板、设置均可滚动；日语长登录标签可见；底部播放控件可达；底部 Library 导航退出沉浸模式。
- 播放中依次切换日语、简中、繁中、英文及深浅色，窗口与播放保持；观测位置从 `187.106666666s` 推进到 `235.757333333s`，没有换歌或重建窗口。
- 官方 Google 登录 identifier 页面可以打开并取消；未输入账号；取消后播放继续，登录 helper PID 消失。
- Cmd+W 后重新观察返回原曲且进度继续；没有独立测量窗口隐藏持续时间。正常退出后 `safe-processes --name ytmgo` 返回空数组，主副本和最小副本都已退出。
- 最小副本启动时曾出现通用网络错误，点击 Retry 后恢复有内容的首页。只证明这次真实错误重试恢复，未建立断网到联网的完整切换证据。

原生截图保存在 [native/](native/)。真实歌词正文、账号标识和登录页临时参数没有写入报告或 JSON。

### 性能观测

本次匿名 cold Load 到非零 PCM 且音频时钟推进为 **14.98s**；包含播放器初始化的冷启动区间约 **16.56s**。三次已预加载切歌首个输出为 **97.97–107.04ms**，每次新增 resolver / decoder 调用均为 0。未预加载随机选择约 **3.58s**。这些是单次网络采样，不是延迟保证。证据见 [anonymous-playback/results.jsonl](anonymous-playback/results.jsonl)。

完整解码不等于整首歌以实时速度从扬声器听完；本轮没有录制声学输出，也没有进行整应用内存基准或长期泄漏判断。

## 授权后的真实 Premium 追加验证

使用前文完全相同的正式身份构建，未重新构建、重签名或更换账号。用户完成系统授权后重启，固定诊断于 `04:34:17Z` 确认为 `premium`，界面账号菜单和设置也均显示 YouTube Music Premium；`04:39:09Z` 后续刷新仍为 Premium。

- 真实音乐库继续加载从 24 项增加到 45 项；艺人筛选及清除筛选可用。喜欢列表、收藏状态、集合播放和队列均可访问。没有执行喜欢 / 取消喜欢、退出或其他云端写操作。
- Premium 实际播放进度推进；暂停位置 21.44 秒，seek 至 110.55 秒成功。切换英文 / 浅色并恢复繁中 / 深色后仍在 110.55 秒暂停，Premium 保留。
- 下一首进入另一首并实际推进到 40.162666666 秒；上一首先归零，暂停在曲首后再次上一首能切回原曲。真实视频帧与非空歌词可显示；关闭视频和面板后音频继续。
- 在已登录状态打开账号管理的临时 Google 窗口，未输入任何凭据；关闭后 Premium 与播放继续。窄元数据追踪的 1 个 helper 和 3 个 WebKit 进程都已退出。
- 普通播放视频阶段的归属快照完整收集 8/8 个进程，包括 3 个 FFmpeg，没有 WebKit。登录阶段 16/16，含临时 WebKit。这里只验证进程生命周期，没有进行内存基准。
- 同一构建退出后，3 个主程序 / runtime 进程全部退出；再次启动，无需重新输入凭据，`04:47:24Z` 重新核验为 Premium。语言与外观恢复为原来的繁中 / 深色，音量 0.7，重启后没有自动播放。
- **曲尾 seek 未通过**：第一首歌曲两次同目标、另一首歌曲一次曲尾 seek 共三次实际超时；Retry 和切换下一首能恢复。不能把上面的成功流程写成 Premium 完整 PASS，详见 F04。

证据：目录 [premium/](premium/)，包括固定状态回执、截图和仅含 PID / PPID / UID / 状态 / 可执行文件的生命周期证据；无账号标识、凭据、媒体地址、argv、环境或原始日志。

## F01 · P2 · 匿名首页分页在线验收未通过

**复现：**匿名 Home 初始返回 2 个 shelves、30 个 items，并提供 continuation。点击 Load more 后没有增加内容，按钮消失。仓库原有 `TestLiveCatalog` 连续两次在 `home continuation returned no items` 失败；补充网络探针和真实 UI 也重现。

网络探针返回 0 shelves、0 items、无后续 cursor。服务响应的可见内容容器为空；只输出结构计数，没有保存响应体或 cursor。额外仅用于诊断的 body-only、query-only 形式同样没有内容。

**证据边界：**这是线上验收失败。尚未区分服务终止页 / 服务策略变化 / 客户端 cursor 选择等原因，不能称为已确认的 Spotify UI 回归，也不能保证一定遗漏了后续歌曲。不应凭这个结果添加 fallback 或伪造内容。

**定位：**`internal/catalog/client.go:218–232` continuation 请求；`internal/catalog/parse.go` 页面解析；`controller.go:478–490` 接受空页并清除继续加载入口。下一步需明确服务终止页语义，再决定修复请求、解析或在线测试断言。

证据：[加载前](native/14-home-load-more-before.jpg)、[加载后](native/15-home-load-more-after.jpg)、[结构探针源码](catalog_shape_audit_test.go.txt)、[results.json](results.json)。

## F02 · P3 · 短桌面窗口的沉浸信息区贴底

**复现：**真实半屏窗口约 960×641，播放 `Daft Punk : Get Lucky // Grammys Rehearsal (HD)` 并进入 Cover 沉浸模式。两行标题占据下方空间，艺人文本紧贴面板底边，下沿缺少留白并有裁切观感。桌面沉浸信息区没有可用的纵向滚动补救；紧凑模式有滚动，已另行通过。

软件 fixture 在 960×609 内容尺寸下也得到艺人框 `Y=512.09863, H=16.901367`，底边恰为 529，即内容高度减 80 高播放条。原有几何断言仍通过，说明仅检查控件在窗口内无法验证文字的最终裁切与留白。

**定位：**`native_views.go:1409–1432`。封面尺寸由剩余高度推算，标题允许两行，但整体列没有根据真实标题高度约束封面或提供滚动。

**建议验收：**在短窗口和长标题组合下，艺人完整显示并保留底部间距；或者信息区可滚动，同时固定播放条可用。通过后重查深浅色、CJK 与视频模式。

证据：[真实窗口](native/12-short-immersive-clipping.jpg)、[软件复现](snapshots/audit-960x609-long-immersive.png)。这是轻度布局问题，不是播放中断。

## F03 · P3 · 真实账号侧栏重复展示类别

**授权后新增发现：**音乐库中多条播放清单显示“播放清單 · 播放清單”，艺人条目显示“藝人 · 藝人”。封面和导航正常，但副标题重复，未提供创建者或其他新信息。

**定位：**`native_views.go:552–559` 将 `nativeKindLabel(item)` 与 `item.Artist` 无条件拼接；真实服务返回的部分 `Artist` 字段本身就是类别文案，因此两次显示同一类型。

**复现与证据：**[真实 Premium 界面](premium/06-premium-video.jpg)。新增只在审计 overlay 中运行的 `TestSpotifyAuditLibraryMetadata`，使用本次真实界面已观测到的模型形态，在 en、zh-Hans、zh-Hant、ja 四语言均检测到重复；四个审计断言失败，生产代码未修改。fixture 截图见 `snapshots/audit-library-metadata-*.png`，源码见 [spotify_audit_test.go.txt](spotify_audit_test.go.txt)。

**建议验收：**类型只出现一次；有真实艺人或创建者时显示有意义的次级信息。不能把类别字符串当作创建者。

## F04 · P2 · Premium 曲尾 seek 在两首歌上共三次实际超时

**授权后新增发现：**在正式身份原包恢复 Premium 后，从喜欢列表播放第一首约 221.1 秒的歌曲。普通播放、暂停、seek 至 110.55 秒均成功，但播放中 seek 到 `218.293646264s` 后，加载结束为 `playback_timeout`，界面显示“暂时无法播放，请重试或选择其他歌曲”，预期的曲尾到下一首没有完成。

点击 Retry 后从头恢复，实际进度推进到 `10.722666666s`；再次对重新解析后的当前歌曲执行同一曲尾 seek，仍超时。两次固定类别回执时间为 `2026-10-29T04:42:54Z`、`04:44:46Z`。这项实际应用失败与 race 测试超时是两件事。

**交叉验证：**切换到另一首 Premium 歌曲后，普通播放推进至 `29.762666666s`；seek 到 `196.316185541s` 同样超时，固定回执时间 `04:46:29Z`。因此本轮为两首歌曲、三个实际失败样本，尚未建立所有曲目 / 所有网络条件下的覆盖。证据：[第二首失败](premium/09-premium-second-track-tail-timeout.jpg)、[固定回执](premium/second-track-tail-failure.json)。

**定位与边界：**`controller.go:495` 的 seek、`internal/player/player.go:470` 的新 decoder 安装和 `internal/player/decoder.go:20,85` 的首批 PCM 30 秒超时路径需要调查。固定诊断只有 `playback_timeout`，没有 HTTP 状态 / 失败阶段；尚未区分该曲目媒体布局、上游网络、流桥接或 decoder 问题，也没有证据表明 Spotify 界面改动导致了它。不能概括为所有 Premium 播放失败。

证据：[首次失败](premium/07-premium-tail-seek-timeout.jpg)、[重复失败](premium/08-premium-tail-timeout-repeat.jpg)、[复现元数据](premium/tail-seek-reproduction.json)、[首次固定回执](premium/tail-seek-failure.json)。本项修复并通过真实尾部 seek 前，Premium 播放完整验收不能签 PASS。

## R01 · race 测试不稳定，未确认产品缺陷

首次 `go test -race ./...` 的 `TestMutedVolumeSurvivesNaturalEndAndPreparedNext` 在 2.00s 超时，位置 `internal/player/volume_test.go:100`。没有 data race 报告。随后定向 `-race -count=10` 通过，完整 race 套件复跑也通过；真实窗口的曲尾切歌与静音保持通过。

首次失败保留为不稳定证据，不能把复跑成功改写成从未失败，也不能据此断定真实播放器存在静音丢失。

## 其余边界与文档差异

- 真实账号恢复与当前 Premium 主要流程已追加；新账号首次登录、账号切换 / 退出、云端写操作未测。诊断操作没有直接读取用户会话文件、钥匙串值、argv 或进程环境；进程诊断仅使用窄元数据收集器。
- 物理媒体键、完整 VoiceOver、真实 CJK IME、睡眠唤醒、长时间后台、明确断网恢复未验收。向 Finder 发送合成全局快捷键没有改变播放，无法区分自动化事件路由与产品注册问题，列为未验证。
- Windows / Linux / Intel / 较老 macOS、CI、安装状态、Developer ID / 公证与公开发布均不在本次通过范围。
- `DESIGN.md:180–191` 仍描述“不允许任何 WebKit / extension handoff”，与实际临时 Wry 登录及 README 不一致；这是文档差异，不作为本轮运行时缺陷。
- 本轮只增加测试报告和证据，没有修改业务代码，也未提交、推送或发布产品修复。
