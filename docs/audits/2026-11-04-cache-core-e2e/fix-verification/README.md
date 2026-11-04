# 修复与复测（2026-11-04—05）

本轮针对前一轮根因报告修复产品实现，并重新执行自动回归、真实匿名网络与签名原生包验收。基线为 `d92e39fb5543619d1a1a5864e976fa308b8e2db9`，改动尚未提交或发布。原有其他审计改动未合并到本次补丁。

**结论：已确认缺陷的修复及核心功能回归通过。** 首帧误超时在后续已稳定复现并修复，见[首帧跟进报告](../first-frame-followup/README.md)。完整验收仍有物理全局快捷键、睡眠唤醒及其他平台实机验证项；没有把重试前的失败记录删除或算作通过。

## 修复

1. **匿名 proof 首页读取**：协议请求统一协商 gzip，压缩前后分别限制为 16 MiB；损坏、未知编码和超限响应均拒绝，不返回半截数据。完整 proof HTML 请求预算为 60 秒，调用者取消仍立即生效。
2. **歌词降级缓存**：临时失败后的可读纯文本继续返回，但不会作为十分钟完整成功缓存。下一次加载可以重新补全时间轴；成功时间轴和确认只有纯文本的结果仍复用，并保留并发合并、账号隔离及私有切片复制。匿名移动端临时失败也适用。
3. **媒体中断恢复**：原生 Go lease 保留每次上游读取 20 秒限制，并对每个分段最多恢复两次响应体故障，只请求尚未交付的字节。范围不符、权限拒绝仍终止。FFmpeg 不再用独立十秒计时器抢先打断原生 lease；兼容本地 relay 使用 90 秒看门狗。音频首个 PCM 的 30 秒截止和显式取消保持有效；视频采用相同媒体输入超时策略。
4. **播放错误分类与 CDN 跳转**：decoder 在发布 EOF 前保存固定错误类别和有界 HTTP 状态码，Snapshot 保留网络超时、权限拒绝、截断等原因，失败不触发正常自动下一首。实际长播再次中断时确认媒体 HTTP 302；修复后媒体范围请求只跟随 HTTPS `*.googlevideo.com` 的跳转，最多三跳，每跳保持相同字节范围且不转发账号头。账号/协议请求仍拒绝重定向。原始 FFmpeg 消息和跳转 URL 不进入 UI 或诊断。
5. **搜索可诊断性及版本**：目录传输错误保留固定分类，仍兼容 `errors.Is(ErrUnavailable)`；原生页面诊断只写类别、allowlist 阶段、状态码和数值耗时，不存查询、响应、URL、账号或认证字段。诊断及验收采样读取实际包版本，消除原先硬编码 0.6.2 与实际 0.6.4 的错配。

没有修改歌词拖动失焦保护；此前原生探针观察到的失焦取消，不足以证明物理鼠标操作算法存在错误。

## 已完成验证

| 验证 | 结果 | 证据 |
| --- | --- | --- |
| 全量 Go 竞态回归 | 1256 项测试/子测试通过，0 失败，21 跳过；12 个有测试的包通过 | [race.json](race.json) |
| Go vet、四语 i18n | 通过；最终 Go 变更后再次检查 | [checks.json](checks.json) |
| 登录、许可、发布、诊断 Python 测试 | 10 / 7 / 38 / 24 项通过 | [checks.json](checks.json) |
| 原生 runtime 协议/DOM/清单及 Rust 测试 | 通过 | [checks.json](checks.json) |
| 包签名及包内许可/对应源码 | Developer ID 签名严格验证通过；明确验证最终包 Resources | [build-v4.json](build-v4.json) |
| 真实 proof HTML | HTTP 200，gzip 272362 字节 → 960875 字节，2296 ms，完整解析 | [proof-home.json](proof-home.json) |
| 真实公共目录缓存 | 搜索与歌词缓存复用通过 | [public.json](public.json) |
| 真实匿名整曲解码 | 最终代码的两首全部到达 EOF、PCM 有效且非零，decoder/bridge 均无失败 | [anonymous-full-final/results.jsonl](anonymous-full-final/results.jsonl) |
| 12 秒受控停顿与取消 | 实际 FFmpeg 完整恢复解码；200 ms 取消仍在 3 秒内完成 | `TestActualDecoderSurvivesTransientStall` / `TestActualDecoderStallRemainsCancelable`，包含于全量竞态回归 |
| 分段故障恢复 | 无字节超时、已有字节超时、断连、截断均精确续读；最多两次恢复；取消及时终止 | `TestLeaseRecoversExactUndeliveredRange` / `TestLeaseCancellationDuringRecovery` |
| 歌词临时失败恢复 | 公共 queue/browse、匿名 mobile 失败后重新获取真实时间轴；成功与确认纯文本仍命中缓存 | `TestLyricsTimingCacheRecoversAfterTransientFailure` |
| 中途错误到 Snapshot | timeout / access / rate / range / truncation / decoder 分类全部保留 | `TestMidstreamFailureCategoryReachesSnapshot` |
| CDN 重定向 | 301/302/303/307/308 保持相同范围与无认证头；拒绝非媒体域、HTTP、端口、用户信息、片段及超过三跳 | `TestMediaCDNRedirectKeepsExactRangeAndNoAccountHeaders` / `TestMediaRedirectBoundaryAndLoopLimit` |
| 媒体 HTTP 元数据 | 403/416/429/503 固定状态与既定重试数保留，任意文本及非法状态被过滤 | `TestLeaseHTTPFailureRetainsActualStatus` / `TestMediaFailureMetadataIsBounded` |
| 官方 MV 实际画面 | 追加独立 `-race` 实测通过；854×480，两个位置共推进 157 帧，抽样像素最大平均差异约 30.35% | [real-mv.json](real-mv.json)、[real-mv-test.json](real-mv-test.json) |
| Premium 连续播放及自然切歌 | 从原 12.65s 位置播放至约 6:09 曲尾，自动切到下一首并继续 95.1s，79 个采样无失败 | [native-summary.json](native-summary.json)、[after-next.json](after-next.json) |
| 五分钟账号检查 | 16:25:19 UTC 再次确认 Premium，当前与预加载解码器 PID 7860 / 7870 均保持不变，时钟继续推进 | [before-check.json](before-check.json)、[after-check.json](after-check.json) |
| 原生歌词拖动释放 | 自动化连接重置后，在实际前台组件中从 15.18s 拖动，释放跳到真实 cue 42.88s，随后音频时钟正常推进 | [native-lyrics-gesture.json](native-lyrics-gesture.json) |

真实匿名整曲结果：212.561 秒元数据对应 212.526667 秒 PCM；369.641 秒元数据对应 369.606667 秒 PCM，均在既定 0.5 秒容差内。

全量回归初轮发现新增测试夹具与 MyGo 的解码 goroutine 并发修改数据；已改为安装输出前构造完整夹具，最终 `-race` 全量通过。原始失败摘要保留于 [race-initial.json](race-initial.json)。增加有界媒体续读前的真实匿名失败也保留于 [anonymous-full/results.jsonl](anonymous-full/results.jsonl)，其固定分类明确为上游读取超时；不能据此重写此前 4:50 事件的历史根因。

## 原生包验收记录

最终测试包：`build/cache-fix-verification-20261104/package-v4/darwin-arm64/ytmGO.app`，可执行文件 SHA-256 为 `2ca7825d68e3da2f588745da665ea975f6f2a539e0259845a2d185f5b6992a60`。使用现有签名身份构建，只通过原应用账号恢复入口恢复 Premium；未读取认证存储。该包未公证，不能代替发布验收。

- Premium 账号、保存的资料库、English / Dark、原音量恢复成功。
- 原失败查询 `P!nk Try Official Video` 在实际登录路径返回歌曲及官方视频；播放时钟持续推进。
- Antidote 专辑正确返回 13 项 Derik Fein 曲目，加载时音频未中断。
- 最终构建持续播放采样：[native-v4-playback.jsonl](native-v4-playback.jsonl)。持续播放、五分钟账号检查及自动下一首均通过；预加载解码器 7870 接管新曲，另建下一首预加载。持续播放采样在后续手动队列/歌词手势操作前停止，因此没有把手动切歌当作自动切歌。
- 真实视频复测通过：两个位置的移动时钟首帧与帧推进通过；实际匿名音频播放、73.125s / 117.625s 跳转、暂停恢复时，每两秒分别推进 47–48 帧，音频约推进两秒，见 [video-final.json](video-final.json)。
- v2 登录长播在 236.534s 出现 `audio_server_error`，五分钟账号检查随后仍为 Premium。该次没有保留具体媒体状态码，不能断言为某个 5xx。故障证据保留于 [native-v2-playback.jsonl](native-v2-playback.jsonl)、[native-v2-audio-failure.json](native-v2-audio-failure.json)。v3 补充封闭的媒体 HTTP 状态元数据后，重新独立长播；不删除或覆盖这次失败。
- v3 在 42.114s 再次中断，固定元数据明确记录 `media_stream / HTTP 302`，见 [native-v3-audio-failure.json](native-v3-audio-failure.json)。这确认此前一直存在的 CDN 重定向缺口；不据此断言最初那次 4:50 历史事件一定同因。v4 加入有界、逐跳验证的媒体跳转，重跑最终整曲验收。

补充 MV 探针首先在单一三秒窗口测到约 1.3% 像素变化，低于原先 3% 判据，见 [real-mv-initial-window.json](real-mv-initial-window.json)。因此改为比较两个独立播放位置，未降低 3% 阈值，最终观察到约 30.35% 差异，公开 MV 抽帧也已视觉检查。另一次运行未取得首帧，见 [real-mv-startup-retry.json](real-mv-startup-retry.json)，当次元数据不足以唯一追溯原因，当时保留为待复现项。后续已确认首帧计时覆盖冷解析的缺陷并修复，受控复现和新验证见[首帧跟进报告](../first-frame-followup/README.md)。

采样中的 `DecoderPIDs` 按候选包运行时路径筛选；匿名 CLI 一度使用同一路径，不应把此时总数作为原生 app 泄漏证据。五分钟检查前后使用额外 PPID 约束核对实际 app 子进程，见对应 checkpoint。

## 收尾

测试 app 及其子进程已退出。原来的 `/Applications/ytmGO.app` 0.6.3 已恢复运行并保持暂停，12 项队列、12.65s 位置、0.659167945 音量及原语言/主题/歌词布局文件全部按备份恢复；可执行文件 SHA-256 仍为 `485d6a62848b1541dcbc5d95657795f2da333591d387304c64d7e73684b3a3fe`。实际 UI 再次确认 Premium、Play 按钮、12.65s 和原音量。验收请求已移除，见 [restoration.json](restoration.json)、[final-state.json](final-state.json)。

计算器前台的工具注入 `Command+Shift+Space` 未观察到播放切换，见 [global-shortcut-probe.json](global-shortcut-probe.json)。该结果没有认证物理全局快捷键，也不足以据此修改原快捷键设置或判定其损坏。

## 可复核性与边界

[source.json](source.json) 固定基线和各修改文件哈希；[tracked-changes.patch](tracked-changes.patch) 保存已跟踪文件补丁，新测试源码位于对应工作区路径。所有诊断仅保留封闭类别、计数、时长、状态和可执行文件身份/PID，不保存原始依赖日志、认证信息或签名媒体 URL。

搜索的历史失败没有足够证据还原；本轮补齐安全分类并复测当前实际登录查询。物理全局快捷键、睡眠唤醒、Windows/Linux 原生设备及长时间运行不由这些测试替代。歌词拖动已在原生窗口工具操作下通过，失焦保护仍有组件回归覆盖。公开 MV 的可选 PNG 抽帧只含测试所选的公开视频，不含账号界面、认证数据或媒体地址。
