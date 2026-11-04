# 歌词拖动同步进度：E2E 验收

2026-11-04，承接任务 `01a1a5a8-72dc-7b42-b522-2b037d6fed45`。**本次 macOS 已安装版的歌词拖动功能验收 PASS，未发现功能失败。** 首次播放发生一次网络超时，Retry 后恢复；异常保留在本报告中，未据此声称整个应用的网络稳定性通过。

本轮没有修改产品源码，没有提交、推送或发布。新增内容仅为本目录的验收记录和可重复验证脚本。运行对象是 `/Applications/ytmGO.app`，版本 0.6.2。最初发现另一个开发构建正在运行，已正常退出后启动安装版，避免测试错误产物。

安装二进制 SHA-256 为 `c8d8ea6746e346a065d02e7f854f7b4e9be052a5ee21b76fb4a8ce2d427e4ac3`，与原任务安装记录一致。原任务记录的八项歌词功能源码指纹在本轮开始、结束时均一致。全量自动测试对应当前工作区；安装版证据绑定原任务的二进制，不将其他工作区改动笼统视为已安装。[基线](baseline.json) · [自动检查](checks.json) · [结构化结论](result.json)

## 原生应用实测

使用 Premium 已登录账号播放《Try》正式歌曲录音。独立匿名查询得到同一录音 `Glt0RUX2Q2Q` 的 20 个上游时间片，只记录索引和时间，不读取账号凭据。[上游时间点](provider-cues.json) 与 [退出时保存的录音 ID](44-restart-saved.json) 一致。

| 场景 | 实际结果与证据 |
| --- | --- |
| 暂停时向前拖动 | 19.210666666 → **55.03 秒**，松开后进入解码准备，完成后仍显示 Play，位置稳定为 55.03。[松开](07-paused-drag-forward.ax.txt) · [稳定状态](08-paused-drag-settled.ax.txt) |
| 暂停时向后拖动 | 55.03 → **51.43 秒**，仍保持暂停，恢复跟随，没有残留预览。[稳定状态](10-paused-backward-settled.ax.txt) |
| 单击、横向拖动 | 都保持 51.43 秒，没有停止跟随或误跳转。[单击](11-click-no-seek.ax.txt) · [横向拖动](12-horizontal-no-seek.ax.txt) |
| 滚轮阅读与再拖动 | 滚轮保持 51.43 秒并显示 Follow playback；随后拖动跳到 **89.89 秒**，按钮消失，恢复跟随。[滚轮](13-wheel-manual-no-seek.ax.txt) · [拖动](14-drag-from-manual-reading.ax.txt) |
| 播放中拖动 | 松开落到 **103 秒**，随后显示 Pause 并推进到 **108.954666666 秒**，继续跟随。[松开](17-playing-drag-release.ax.txt) · [继续播放](18-playing-drag-advanced.ax.txt) |
| 拖出歌词区域再松开 | 从歌词拖至封面区域松开，成功落到最后一个时间片 **155.48 秒**，仍暂停。[记录](19-outside-release-last-cue.ax.txt) |
| 首句边界 | 手动滚至顶部，再向下拖动，钳制到上游首片段 **0 秒**，没有负进度。[记录](21-first-cue-boundary.ax.txt) |
| 标准与放大歌词布局 | 宽窗口两种布局都实际操作；放大布局前后拖动和标准布局播放中拖动均通过。[宽窗口标准](16-standard-playing-before-drag.jpg) · [放大布局](07-paused-drag-forward.jpg) |
| 紧凑浮层 | 962×902 逻辑像素浮层从 0 跳到 **51.43 秒**，外层内容保持位置。[操作前](23-compact-lyrics-visible.jpg) · [操作后](24-compact-drag.jpg) |
| 窄而短的真实窗口 | 系统窗口命令取得 **757×641** 逻辑像素；标准布局拖至 **55.03 秒**，放大布局拖至 **59.45 秒**，外层页面保持稳定。[标准布局](28-narrow-standard-drag.jpg) · [放大布局](30-narrow-expanded-drag.jpg) |
| 沉浸模式 | 窄窗口滚动到歌词后，59.45 → **64.2 秒**，歌词正确居中，外层未跟着拖动。[操作前](32-immersive-lyrics-visible.jpg) · [操作后](33-immersive-drag.jpg) |
| 切歌与无歌词回退 | 换到另一个 Try 歌词视频录音，旧歌词被清除，显示无歌词提示；拖动前后均为 **4.533333333 秒**，没有拖动提示或错误 seek。[操作前](37-unavailable-visible.ax.txt) · [操作后](38-unavailable-drag-no-seek.ax.txt) |
| 拖动断点跨重启 | 回到正式歌曲并拖至 **64.2 秒**；正常退出后保存的时间精确一致，冷启动及 Premium 恢复后仍在 64.2 秒暂停。[保存记录](44-restart-saved.json) · [恢复后](46-restart-account-restored.ax.txt) |

各实机跳转值均与匿名查询的上游 cue 起点一致。截图中的选中歌词完整可见；普通、放大、沉浸布局的外层位置通过操作前后截图复核。

## 自动化与真实解码器补验

- `go test -race -json -count=1 ./...`：**1,120 项通过、28 项跳过、0 项失败**，计数包含子用例，耗时 181.243 秒。[逐项结果](go-test-cases.json)
- 歌词拖动有六个顶层测试，含按下期间不 seek、多次移动后只提交最后选中句、释放一次只 seek 一次、播放/暂停保持、区域外释放、首尾钳制、Escape/失焦/缩放/切歌/歌词更新/加载取消、无效时间轴、点击抖动、横向动作和换行行高。另有 **40 个布局/语言/明暗组合**。以上取消手势和多段移动属于 UI Tester 证据，未混称为实体鼠标实测。
- 本轮生成并目检 `build/lyrics-drag-e2e-20261104/snapshots/lyrics-drag-preview-240.png` 和 `lyrics-drag-420x640_true_false_zh-Hans_light.png`：选中句、中心线和“松开跳转”时间预览可读。420×640、240 宽属于确定性 UI Tester；实机最窄窗口为 757×641。
- 补跑安装包内真实 FFmpeg 的五个顶层测试：AAC、fragmented AAC、Opus 的录音时间轴跳转，末尾范围读取及 PCM 一致性，末点暂停/自然结束、实际频率跳转和 EOF 重启。**26 项含子用例全部通过，无跳过**，耗时 14.285 秒。这五项在普通全量测试中需要显式运行时路径，现已单独启用并补验。[解码器结果](decoder-checks.json) · [复现脚本](run_decoder_checks.py)
- `go vet ./...`、四语言翻译检查、运行时清单/协议/测试、登录 helper 测试、许可测试、`git diff --check` 和安装包 `codesign --verify --deep --strict` 全部通过。[检查脚本](run_checks.py)
- [verify_evidence.py](verify_evidence.py) 对原生 AX 的精确落点、暂停/播放推进、误操作保护、上游时间轴、重启保存、原状态恢复、源码和安装指纹执行断言，结果见 [result.json](result.json)。

## 异常与覆盖边界

首次 Try 播放失败：`bootstrap_network_timeout`，阶段 `bootstrap`、步骤 `player_script`。当时歌词退回纯文本。点击 Retry 后同一歌曲成功播放并加载同步歌词，后续所有拖动和重启用例完成；最后一次播放器诊断为 `ready`。[首次失败类别](03-initial-playback-status.json) · [恢复后](04-retry.ax.txt) · [末次类别](47-final-audio-status.json)。没有排查或修改网络配置，根因仍未确定。

本轮保留用户原来的静音状态。真实歌曲的播放证据是原生播放器及解码时钟推进，另有合成音频的实际解码 PCM 校验；没有声学回环录音，不声称验证了扬声器听感。未测试实体触控板、多指手势、操作系统层面按住鼠标时失焦或按 Escape（相关状态机由 UI Tester 覆盖）、睡眠唤醒、网络故障注入、账号切换、Windows/Linux、长期稳定性或公开发布包公证。

坐标校准时有一次拖动落在搜索列表，未改变播放断点，`06-paused-drag-forward` 只保留为工具操作记录，不计为歌词通过证据；之后按截图像素在正确歌词区域完成所有用例。原生边缘缩放部分命令未改变窗口尺寸，最终通过系统 Window 菜单取得窄窗口，尺寸从图像元数据核验。

## 恢复用户状态

测试结束后正常退出，并通过 `safe-processes` 确认应用已停止，再恢复测试前保存的播放文件。原 **25 首队列及顺序、index=1、29.765 秒、音量 0 和取消静音音量**逐项一致。重新启动已安装版，Premium 恢复后仍保持暂停，窗口恢复宽布局、歌词区域保持原来的放大选项。[恢复校验](49-original-restoration.json) · [最终状态](50-final-original-state.ax.txt)

诊断仅采集 `safe-processes` 的 PID/PPID/UID/状态/可执行路径及源码定义的固定错误类别，没有读取进程 argv、环境值、认证存储或原始诊断日志。自动检查脚本只落盘经过允许字段筛选的结果元数据。
