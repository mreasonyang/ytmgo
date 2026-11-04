# 缓存修复与核心功能 E2E — 2026-11-04

**最终结论：未通过完整验收。** 本次缓存改动的多数主要行为已验证，但补测确认了一个歌词降级缓存缺陷；真实网络下还观察到匿名初始化失败、搜索失败和一次播放中断。用户解锁后已补验歌手页面并恢复正式版。同步歌词拖动未能通过本轮工具操作验证，不能计为通过。

后续已完成失败原因调查：[根因、对照实验与修复方向](rootcause/README.md)。匿名首页压缩/预算问题、音频预算冲突与错误分类丢失均已复现；真实歌词组件复现出工具操作释放时无焦点，触发正确的失焦保护。历史搜索失败和那一次音频中断的具体上游触发事件仍不能还原。

测试时间为 2026-11-04 晚间（Asia/Shanghai）；JSON 证据通常使用 UTC。

## 测试对象与方法

- 本次改动：`0882bff` 公共播放器 JS 持久缓存，以及 `d94b40d` 账号检查、预加载、头像、页面、队列、歌词与公共验证脚本复用。
- 源码检查起点：`3659006`。其相对 `d94b40d` 的后续改动属于发布流程，不改变本轮播放器和目录实现。测试期间另一项工作推进到 `d92e39f`，仅增加发布文档和证据，未修改本轮核心实现；没有回退或混入该项工作。
- 真机使用 `d94b40d` 已构建的 0.6.4 arm64 包。原始可执行文件 SHA-256 为 `a117238f4a6b0824d9e1bdc672a1e92bdeb2e8475e1d41ab0559a170b46cf4cb`，与原构建回执一致，见 [baseline.json](baseline.json)。不要将同目录较旧的外层 `.app` 当成本次测试包。
- 原始包为临时签名，无法静默恢复已保存账号。复制后仅使用现有 Developer ID 重新签名，没有重新编译或读取账号凭据；嵌套签名、资源清单与严格签名检查通过，见 [signed-test-bundle.json](signed-test-bundle.json)。此副本没有公证，不是发布验收。
- 实机操作通过内置电脑控制工具完成；进程只通过 `safe-processes` 按可执行文件身份采集。网络探针只保留次数、时长、状态码、字节数和固定类别，没有保存请求内容、凭据、令牌、媒体 URL 或原始诊断日志。

## 本次改动验收

| 场景 | 结果 | 证据与边界 |
| --- | --- | --- |
| 首次实机播放生成公共 JS 缓存 | 通过 | 测试前不存在；播放后生成 199,144 字节、0600 权限记录，见 [cache-first-playback.json](cache-first-playback.json) |
| 实际退出、重启后复用 JS | 通过 | 主进程 PID 更替；从 216.768 秒恢复并继续播放，缓存 mtime 未变，见 [before-restart.json](before-restart.json)、[restart-playback.json](restart-playback.json) |
| 真实公共 JS 下载、提取与磁盘恢复 | 最终通过，存在瞬时失败 | 最终首次 4.958 秒；新 resolver 恢复 1 ms，测试禁止其联网或启动提取器，见 [live-js-final.json](live-js-final.json)。较早探针失败为 `bootstrap_network_timeout`，见 [live-js-retry.json](live-js-retry.json)，不能声称冷启动完全稳定 |
| 不变账号定时检查保留有效预加载 | 通过 | 实机账号诊断时间从 22:48:16 更新到 22:53:14；当前与下一首 FFmpeg PID 始终为 82059、82144，播放持续推进，见 [periodic-account.jsonl](periodic-account.jsonl) |
| 重复搜索与歌词不新增请求 | 通过 | 真实搜索首次 1 次、重复 0 次，32 项；歌词首次 2 次、重复 0 次，见 [live-catalog-retry.json](live-catalog-retry.json)。此样本没有时间轴，不将其当作真实同步歌词缓存证据 |
| 同曲请求合并、同步歌词补全结果复用 | 自动化通过 | 全套 race 检查包含本次 fixture：三个消费者共享一次 next、时间轴深复制与补全缓存、负结果短缓存 |
| 强制刷新、过期、损坏、版本变化、写盘失败 | 自动化通过 | player/cache/proof 回归用例通过；未等待六小时或逐项修改真实用户缓存进行实机故障注入 |
| 账号/语言隔离、迟到响应、取消、容量、点赞失效 | 自动化通过；语言实机通过 | race 检查覆盖作用域与取消；四种语言在实机中依次切换，最后恢复英语 |
| 公共验证脚本跨环境/重启复用 | fixture 通过；真实链路受阻 | 两轮真实探针均在 YouTube 首页读取失败，尚未执行到 interpreter 请求，因此不能用它们证明真实脚本缓存或新令牌生成，见 [proof-lifecycle.json](proof-lifecycle.json)、[proof-lifecycle-retry.json](proof-lifecycle-retry.json) |
| 头像请求复用 | 自动化通过 | 实机 Premium 头像可见；没有采集 GUI 头像请求次数，30 分钟过期依赖 fixture |

## 核心功能实机结果

| 场景 | 结果 |
| --- | --- |
| 已保存 Premium 账号、头像、资料库恢复 | 通过；重启时先显示禁用的本地快照，验证成功后恢复可操作状态 |
| Home、Explore、Library、专辑、筛选 | 通过；Antidote 显示 13 首曲目；专辑与歌手筛选可切换 |
| 搜索 | 部分通过；Try P!nk 两次返回结果，之后视频搜索及一次 Retry 均失败 |
| 专辑页面前进/后退 | 通过；Home → Antidote 恢复，音频不中断 |
| 歌手详情 | 补测通过；Dreamkid 图片、热门歌曲、专辑、单曲、视频、歌单与相似歌手均加载 |
| 播放、暂停、恢复 | 通过；暂停 12 秒位置保持不变；恢复后音频时钟推进 |
| 进度条 seek | 通过；指针点击跳到 255.031 秒，保持暂停，随后继续播放 |
| 下一首、队列回选、Previous | 通过；下一首在同次观察中进入播放；Previous 在当前曲已播放三秒后重置当前曲，属于现有规则 |
| 自动切歌 | 通过；369 秒原曲结束后自动进入 248 秒 Official Audio，并继续超过两分钟 |
| 点赞/取消点赞 | 通过；服务端成功后按钮改变，最后恢复原曲原来的点赞状态 |
| 随机、循环、静音 | 通过；队列尾部重排；循环 off/all/one/off；暂停时恢复静音前 0.659167945 音量，再恢复静音 |
| 同步/纯文本歌词 | 通过展示；原曲曾有 110 条时间轴，下一曲明确提示时间轴不可用。补测纯文本拖动前后均为 25.493333333 秒；固定同步录音的多次工具拖动前后均为 9.76 秒，未观察到跳播，不计为通过；手动滚动与恢复跟随正常 |
| 视频、沉浸模式、关闭视频释放资源 | 通过显示和开关；关闭后 FFmpeg 从三个恢复到两个。素材为静态封面，动态 MV 未验收 |
| 四种语言、明暗外观 | 通过；最终恢复英语、深色 |
| 应用内 Command K、Space | 通过；Space 在搜索输入失焦后暂停，输入框聚焦时保留文字输入行为 |
| 全局 Command Shift Space | 未通过自动注入验证；Finder 中注入后播放仍继续，不能据此断定物理按键或注册机制的产品根因 |
| 正常退出与重启状态恢复 | 通过；首次退出后应用与播放进程全部结束；队列顺序、静音、位置恢复 |
| 真实紧凑窗口 | 未验收；边缘拖动没有改变窗口尺寸。小尺寸布局仅有现有原生 UI Tester/race 用例覆盖 |

实机数值记录见 [ui-observations.json](ui-observations.json)。本轮保持测试音频静音；观察到的是原生播放时钟和解码进程，不是声学回环或扬声器听感验收。

## 未通过项

1. **匿名播放初始化失败。** 使用实际签名包的 runtime 做完整匿名解码，在 `proof_init` 返回 `resolver_http_error`，两首整曲解码尚未开始，见 [anonymous-full-decode/results.jsonl](anonymous-full-decode/results.jsonl)。进一步隔离的真实 proof 测试收到首页 HTTP 200，但读到 739,916 字节后失败，未请求公共验证脚本。当前证据可定位为首页响应体读取失败，尚不能确认网络、上游或请求预算各自的责任。
2. **Premium 续播中断一次。** 重启后在 290.934 秒停止，固定诊断类别为 `audio_stream_failed`，见 [mid-track-failure.json](mid-track-failure.json)。用户界面 Retry 可从该位置恢复，随后进度达到 321.742 秒并继续到自动切歌。恢复有效，但不能抵消稳定性失败。
3. **搜索真实网络路径不稳定。** 初期搜索和独立目录探针成功；后续 P!nk 视频搜索及 Retry 显示加载失败，当前播放恢复后该页面错误仍在。未将该失败归因于缓存逻辑。
4. **诊断版本仍为硬编码 0.6.2。** 实际测试包与设置页为 0.6.4；`diagnostics.go` 与 acceptance probe 使用 `main.go` 常量。因此本报告以构建 SHA、包版本、PID 和时间绑定证据，不能只用诊断文件 Version 判断被测版本。
5. **[P2，已隔离复现] 歌词时间轴补全的一次临时失败会固化为十分钟纯文本缓存。** `internal/catalog/lyrics.go` 在公共时间轴查询失败后返回纯文本且 error=nil；外层把它作为成功结果缓存十分钟。同曲再次加载不会重试补全。隔离服务首先返回 signed-in 纯文本并让公共补全返回 503，随后恢复上游：重复加载仍为 **0 条时间轴、0 个新请求**；同一个 client 显式 `Refresh` 后立即得到 **1 条时间轴**，见 [degraded-lyrics-cache.json](degraded-lyrics-cache.json)。期望恢复的独立回归探针返回 FAIL，见 [degraded-lyrics-cache-test.json](degraded-lyrics-cache-test.json)；源码保存在 [degraded_lyrics_cache_test.go.txt](degraded_lyrics_cache_test.go.txt)，未留在产品测试目录。这与“真实无时间轴”和直接返回错误的失败缓存策略不同。本轮只验证并记录缺陷，没有修复实现。
6. **同步歌词拖动未通过最初的工具实机验证；后续在真实组件中定位到焦点条件不满足。** 固定公开录音已经展示时间轴与拖动提示；最初多次操作均未改变 9.76 秒位置。后续隔离原生组件的采样记录完整 down/move/up、有效选中第 12 条，但释放时 `focused=false`，因而被 `scroll.Focused()` 保护取消。见 [后续调查](rootcause/README.md)。这是此前自动化假阴性的具体解释，不能计为物理鼠标已确认的产品缺陷；完整应用在明确保持前台焦点后的验收仍需补测。

补测记录见 [continuation-ui.json](continuation-ui.json)。搜索得到的 Try 实际为 `Bk0PL3fmMdw`；另用常规播放恢复文件构造固定公开录音 `Glt0RUX2Q2Q` 的受控夹具，通过真实播放器加载其真实歌词，移除搜索结果歧义。夹具中的标题/歌手没有经过目录核对，因此仅用于手势验收，不作为曲目匹配、P!NK 录音或封面验收证据。相关方法见 [known-recording-fixture.json](known-recording-fixture.json)，退出时的 ID 与位置见 [continuation-final-test-quit.json](continuation-final-test-quit.json)。

## 自动化与资源验证

- `go test -race ./...`：**1176 个测试/子测试通过，0 失败，30 跳过**；12 个有测试的包通过。跳过包含需要显式选择的真实账号、网络和平台条件，不能视为通过，见 [go-race.json](go-race.json)。
- 解锁后的独立歌词降级恢复回归探针为 **FAIL**，暴露了上述新增覆盖缺口；因此不能将先前 1176 项通过概括为所有缓存边界通过。
- 原生 Rust/Go runtime、登录 10 项、发布脚本 38 项、许可 7 项、安全进程统计 24 项、`go vet`、四语言目录检查通过，见 [checks.jsonl](checks.jsonl)。
- 默认旧构建目录的 `--verify-build` 失败；指定本次原始和重新签名测试包的 `--bundle .../Contents/Resources` 均通过。旧目录失败不是本次测试包资源失败。
- 实际网络失败与复测均保留；只保留经过 allowlist 的结果，汇总器为 [summarize_go.py](summarize_go.py)。公开 proof 探针源码作为 [proof_lifecycle_live_test.go.txt](proof_lifecycle_live_test.go.txt) 保存，未留在产品源码测试目录。
- 补采主程序及关联进程 478 次，有效完整样本 476 次，混合 UI 活动中位数 321 MiB、峰值 514 MiB，见 [memory-summary.json](memory-summary.json)。此采样包含切歌、设置、语言切换，不用于和 MixMusic 做公平对照或判断泄漏。

本轮没有修改产品实现，没有发布、推送、替换 `/Applications` 正式版。未进行真实普通账号/账号切换、Windows/Linux/Intel 硬件、睡眠唤醒、强杀恢复、长时间稳定性、公证发布包或自动更新验收。

## 收尾

Mac 锁屏后已通过可执行文件身份再次核验，仅停止本轮测试包进程；没有遗留本轮测试进程。已恢复备份的 12 项队列，并按最初正式版 UI 观察值恢复 12.65 秒、0.659167945 音量、英语和深色；临时 acceptance 请求已移除，见 [restoration.json](restoration.json)。最初遗留的较旧测试实例在退出时曾覆盖播放文件，因此位置与音量使用最初正式版 UI 的实际值恢复，而没有照抄该旧实例的 0 秒/静音值。

用户解锁后已完成补测：歌手页通过，纯文本拖动无误跳播；同步歌词拖动未通过工具实机验证，见上方具体结果。补测程序正常退出后没有遗留测试进程；再次恢复状态文件并启动 `/Applications/ytmGO.app`。正式版设置页为 0.6.3、Premium、English、Dark，主界面为暂停、12.65 秒、音量 0.659167945，资料库恢复。出现 0.6.4 更新提示时选择了 Remind Me Later，自动安装选项保持未勾选。正式版可执行文件 SHA-256 与测试前一致，没有安装更新或改写正式版。


Follow-up (2026-11-05): [Fixes and verification](fix-verification/README.md).
