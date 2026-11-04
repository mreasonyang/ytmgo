# 失败原因定位 — 2026-11-04

本轮只做诊断和隔离复现，没有修改产品实现、正式版或用户网络配置。临时测试源码均移到本目录；无账号指针/歌词探针已退出。

## 结论

| 失败项 | 定位结果 | 证据强度 |
| --- | --- | --- |
| 匿名初始化 | 首页强制不压缩，完整读取受 20 秒总截止时间限制；当前路由下超时 | 真实请求 ABBA 对照确认 |
| 时间轴丢失后一直纯文本 | 公共补全失败被返回为成功纯文本，再被缓存十分钟，后续同曲不会重试 | 源码与隔离恢复 fixture 确认；本次 d94b40d 引入 |
| 中途播放失败 | 解码器 10 秒读超时早于 Go 上游 20 秒预算，且中途失败分类被统一覆盖 | 预算冲突与分类丢失均已独立复现；历史那一次具体上游事件无法还原 |
| 歌词拖动未跳播 | 在真实歌词组件中记录到移动和有效选中，但释放时没有焦点，保护逻辑取消提交 | 原组件原生窗口复现；支持此前自动化焦点导致假阴性的解释，不作为已确认算法故障 |
| 搜索失败 | 此轮原查询及对照全部成功；历史请求没有足够的安全分类记录 | 历史原因仍未确定，不能指定为 DNS、代理或上游某一方 |

## 1. 匿名首页：强制 identity 与 20 秒总预算组合

`internal/streaming/network.go:64` 默认整个请求预算 20 秒，包含响应体读取；`:84` 只为播放器脚本启用 gzip，其余请求强制 `Accept-Encoding: identity`。匿名 proof 初始化先读取 `www.youtube.com/` 的完整页面，之后才解析和请求 interpreter。

沿用生产 transport、相同用户代理/语言/请求和相同 20 秒预算，仅改变压缩请求，按 identity/gzip/gzip/identity 顺序测量：

| 请求 | HTTP | 总耗时 | 传输字节 | 解码后字节 | 结果 |
| --- | --- | ---: | ---: | ---: | --- |
| identity 1 | 200 | 20.002 s | 782,765 | 782,765（不完整） | deadline |
| gzip 1 | 200 | 12.655 s | 271,824 | 957,916 | 完整读取且 proof 页面解析成功 |
| gzip 2 | 200 | 6.288 s | 265,139 | 913,930 | 完整读取且 proof 页面解析成功 |
| identity 2 | 200 | 20.001 s | 930,089 | 930,089（不完整） | deadline |

见 [proof-home-encoding.json](proof-home-encoding.json)、[诊断源码](proof_network_test.go.txt)。这与 E2E 的首页 HTTP 200、读到 739,916 字节后失败相符。失败发生在 interpreter 前，与公共 interpreter 缓存命中无关。

此问题在 `0882bff` 前已存在；该提交延长了 base.js 预算，没有改变 proof 首页仍为 20 秒且不压缩的行为。另一个冷启动风险是版本 ID 请求只有 5 秒，但没有把该风险当成此次 proof 首页失败的原因。

修复方向：按响应类型支持有界 gzip 解压，并为 HTML 全量读取设置合理的独立预算；保留取消能力和压缩前后大小限制。完成首页读取后仍需重新验证后续 proof 和整曲播放，本次 AB 对照不是完整匿名播放通过。

## 2. 歌词：临时降级被当作完整成功缓存

`internal/catalog/lyrics.go:61` 给公共时间轴补全六秒，`:63` 和 `:65` 的失败不会向外返回；`:75` 返回纯文本与 nil error。`LyricsWithTiming` 在 `:41` 将这个结果保留十分钟。

已有隔离测试先让公共补全返回 503，再恢复服务：重复加载依旧 0 条时间轴、0 个新请求；同 client 显式强制刷新立即取得 1 条时间轴。见 [degraded-lyrics-cache.json](../degraded-lyrics-cache.json)。`d94b40d` 前没有这层结果缓存，重复加载会重新补全，因此十分钟内无法恢复属于本轮新增回归。

修复方向：区分“确认没有时间轴”和“补全暂时失败”；保留可读文本，同时让降级结果不缓存或仅短期缓存，并让用户重试可以重新补全。

## 3. 音频：预算冲突，且诊断原因在中途丢失

`internal/player/decoder.go:95` 给 FFmpeg 设置 `-rw_timeout 10000000`，即十秒。Go lease 每次上游读允许二十秒（`internal/streaming/lease.go:304`），网络响应前也有较长重试预算。解码器可能先退出，使后面的等待和重试失去作用。

用实际测试包的 FFmpeg 读取同一份本地 AAC fixture，只在流上插入延迟，不访问账号或真实媒体：

- 1 秒上游停顿：约 2.030 秒取得 PCM，成功。
- 12 秒上游停顿：约 10.203 秒失败，`audio_network_timeout`；上游尚未恢复，下游已经取消。

见 [audio-stall-budget.json](audio-stall-budget.json)、[源码](audio_stall_test.go.txt)。这证明十至二十秒的瞬时卡顿能提前终止播放，不能证明 E2E 在 290.934 秒时一定发生了同样的卡顿。

中途故障还有独立的可观测性缺陷：`pcmDecoder.Read` 最后只保留 failed 布尔值；`Player.Snapshot()` 在 `internal/player/player.go:561` 将其统一写成 `audio_stream_failed`，没有保留 bridge 的固定分类。注入 `audio_network_timeout`、`audio_access_denied`、`audio_stream_truncated` 后，三者对外都变成相同字符串，见 [audio-category-loss.json](audio-category-loss.json)、[源码](audio_category_test.go.txt)。

因此不能从那份旧诊断文件恢复历史事件的具体原因。修复方向是协调解码器与 Go 的预算，并透传既有固定错误类别；无需记录媒体 URL、凭据或原始 FFmpeg 日志。

## 4. 拖动：实际组件在释放时没有焦点

另建无账号原生诊断程序，并在隔离副本中渲染当前真实 `nativePanelCard`/歌词组件，只增加封闭数值字段的输入和释放采样。数据是明确标识的 synthetic cues，不作为音乐、账号或真实歌词提供方验收。

原组件收到完整的左键 down/move/up，最后释放状态为：

```json
{"can_seek":true,"focused":false,"moved":true,"selected":12}
```

见 [lyrics-native-events.jsonl](lyrics-native-events.jsonl)。`native_lyrics_drag.go:101` 明确要求 `scroll.Focused()` 才提交，防止失焦时的合成释放导致误跳播。MyGo 的 Focused 同时要求元素和窗口有键盘焦点。因此这个原生组件复现已经能解释工具操作时“选中了却不跳”的现象。

通用指针探针还观察到窗口焦点随操作上下文变化，另一轮释放时可以为 true，见 [gesture-events.jsonl](gesture-events.jsonl)。不能将某一次工具操作的焦点状态泛化为物理鼠标一直失败，也不能据此去掉失焦保护。应先让自动化明确保持目标窗口为前台，再补完整应用实机验收。

探针源码：[真实组件入口](lyrics_probe_main.go.txt)、[只添加采样的组件副本](lyrics_drag_instrumented.go.txt)、[通用指针探针](gesture_probe_main.go.txt)。没有修改仓库中的产品组件。

## 5. 搜索：当前可复现性不足，不能补写历史根因

本轮使用生产目录 transport 发出四次新的匿名请求，包含原查询 `P!nk Try Official Video` 两次，以及 `Try Pink`、`Get Lucky` 对照。全部 HTTP 200，约 0.993–1.983 秒完成，返回 31–33 项，见 [search-network.json](search-network.json)、[源码](catalog_network_test.go.txt)。

原请求是在真实登录 GUI 中发生的，本轮匿名成功不能完全排除账号路径差异；也不能抹掉原失败。目录客户端当前会把部分传输错误折叠为 `ErrUnavailable`，界面又展示统一错误，而原 E2E 没有保存阶段、状态码与数值耗时。现有证据不足以还原当时是请求超时、HTTP 拒绝还是连接异常。

后续需要与音频一样补充固定错误分类和数值指标，再在实际登录路径复测；不需要采集原始请求、Cookie、令牌或响应正文。

## 收尾

诊断源码与结果均保留在此目录。临时包只位于 `build/cache-failure-rootcause-20261104/`，两个无账号探针都已退出。正式版仍为原来的 0.6.3、可执行文件哈希未变，见 [final-state.json](final-state.json)。本轮未修复或发布这些问题。


Follow-up (2026-11-05): [Fixes and verification](../fix-verification/README.md).
