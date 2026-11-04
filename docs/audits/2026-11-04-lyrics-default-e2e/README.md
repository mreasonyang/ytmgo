# 默认歌词展示：安装与 E2E 验收

2026-11-04（Asia/Shanghai），承接聊天 `01a1a283-af39-78e0-b07a-60b4b0575fc2`。已将签名本地构建安装至 `/Applications/ytmgo.app`。**歌词相关主流程通过，后续已补齐 757×641 紧凑窗口实机验收。** 全局快捷键仍未通过实机验证；补验另遇到一次 `audio_stream_failed`，Retry 后恢复，根因尚未确定，完整应用验收仍不能宣称全部通过。[最新补验报告](retry/README.md)

本轮没有修改产品源码，没有提交、推送或发布。构建包含基线 `7ca716b0a2b98b2065c7b173a74e3276739652e1` 上构建时已有的全部工作区修改，包括歌词展示、滚动和封面相关改动。初轮验收前后相关源码指纹一致；后续补验期间工作区另有变化，安装产物保持不变，详见补验报告。下文自动回归和初轮 `result.json` 对应初轮源码，最新结论见 [retry/result.json](retry/result.json)。

## 安装与产物绑定

- 版本仍为 0.6.2，属于本地验收构建；Developer ID 签名通过 `codesign --verify --deep --strict`，未重新公证。
- 安装二进制 SHA-256：`cd91528fd113aeec09ffa522372a8f5c3bb7c168349e818c47dbfe874a2631c9`。
- 构建目录：`build/lyrics-default-e2e-20261104/candidate/darwin-arm64/ytmgo.app`。
- 旧安装副本已完整保存在 `build/lyrics-default-e2e-20261104/installed-before/ytmgo.app`；在旧进程结束后移开旧包，再完整复制新包。
- 新安装的 178 项文件/链接与候选产物一致，验收结束后再次逐项验证。运行路径通过 `safe-processes` 精确匹配 `/Applications/ytmgo.app/Contents/MacOS/ytmgo`。

[构建记录](build.json) · [安装记录](installation.json) · [安装清单](installed-manifest.json) · [源码指纹](source-hashes.json) · [结构化结论](result.json)

## 本次歌词修改的原生应用验收

| 场景 | 观察结果 |
| --- | --- |
| 首次播放自动展示歌词 | 从冷启动恢复的暂停歌曲点击 Play，自动展开 Now Playing 并选中 Lyrics。同步歌词与 Musixmatch 来源加载成功，自动跟随期间没有 Following playback 或 Follow playback 按钮。[加载状态](03-first-play-loaded.ax.txt) · [宽窗口截图](05-wide-paused.jpg) |
| 自动跟随与暂停、跳播 | 《Can't Help Falling In Love》从开头继续播放，位置从约 0.7 秒推进至 12.25 秒；远距离跳播仍保持跟随。《Try》在 176.281486805 秒恢复跟随后，当前高亮句完整可见。[播放采样](09-follow-playing.ax.txt) · [恢复后截图](20-mouse-resume.jpg) |
| 偶发输入 | 暂停并跳到开头后，点击歌词、水平滚动命令、顶部继续上滚，都没有产生恢复按钮或改变断点。[AX 记录](06-incidental-input.ax.txt) |
| 手动阅读与恢复按钮 | 实际向下滚动才出现底部 Follow playback；Tab、Return 可恢复跟随，按钮消失且焦点回到歌词滚动区；鼠标点击恢复也通过。[手动状态](07-manual-scroll.jpg) · [键盘恢复](08-keyboard-resume.ax.txt) |
| 手动阅读期间跳播 | 《Try》在约 27.19 秒暂停并手动下滚，随后跳至 176.28 秒，保留手动阅读和恢复按钮；点击恢复后才回到当前句。[手动阅读](18-try-manual.jpg) · [跳播状态](19-manual-seek.ax.txt) · [恢复状态](20-mouse-resume.ax.txt) |
| 手动选择队列 | 从歌词切到 Queue 后点击 Next，歌曲改变而面板继续显示队列。[记录](11-next-queue-selected.ax.txt) |
| 手动关闭面板 | 关闭面板后下一首、暂停和继续播放都没有重新弹出。另一次冷启动中，先在暂停状态打开 Queue 再关闭，随后首次播放加载完成也保留关闭选择。[关闭后切歌](12-dismiss-next.jpg) · [播放前关闭后继续播放](36-preplay-dismiss-loaded.ax.txt) |
| 无歌词、切歌清理 | 从有同步歌词的《Contact》切到《Voyager》，清除上一首内容并显示 Lyrics are not available for this track.，没有无效跟随按钮。[记录](28-voyager-lyrics.ax.txt) |
| 沉浸式界面 | 可进入、退出沉浸模式，歌词保持可读和跟随；Cover/Video 可切换，视频流完成加载并呈现对应画面，播放位置继续推进。本次选用的歌曲视频流呈现专辑静帧，不据此声称验证了动态 MV 画面。[沉浸式截图](21-immersive.jpg) · [视频模式](22-video-playing.jpg) |

初轮原生应用窗口覆盖约 1920×995/996 逻辑像素；后续已补验 961×641 短窗口及 757×641 紧凑窗口。420×640 的确定性测试仍仅属于原生 UI Tester 证据。

## 主流程回归与恢复原状态

- 冷启动自动恢复 Premium、头像、曲库和封面，首页加载正常；搜索 `Try Pink` 并选择 Song 类型录音、搜索并播放 Contact/Voyager、进入 Discovery 专辑及读取完整曲目列表均完成。[搜索](16-search-results.ax.txt) · [专辑](29-album.ax.txt)
- 歌曲结束后队列自动继续到下一首；手动 Next 和应用内切歌快捷键可用。静音使音量变为 0，取消静音恢复 0.7。设置保持 Dark、English、0.6.2。[设置截图](24-settings.jpg)
- 播放《Voyager》期间正常退出，确认没有 ytmgo 相关进程存活；保存第 8 项、45.389333333 秒、70% 音量和 11 首队列。冷启动精确恢复相同断点、相同音量并保持暂停，Premium 恢复后仍不自动播放；在原生队列中逐项核对 11 首顺序。[退出记录](31-restart-saved.json) · [暂停恢复](32-restart-paused.ax.txt) · [完整队列](33-restored-queue.ax.txt)
- 验收结束后正常退出，确认进程结束，在停止期间恢复测试前的播放文件，再启动安装版。最终为《Can't Help Falling In Love》、原 90 首队列、index=8、162.466666666 秒和 70% 音量，保持暂停，Premium 已恢复。最终启动没有预先打开或关闭 Now Playing，保留下一次首次播放自动展开歌词的默认行为。[最终截图](41-original-state-restored.jpg) · [最终播放及进程元数据](42-final-process-and-session.json)

[verify_evidence.py](verify_evidence.py) 对源码和安装指纹、跟随按钮状态、位置推进、暂停恢复、队列顺序、进程更替和恢复原状态作断言；执行通过，结果见 [result.json](result.json)。播放依据为原生播放器状态和解码位置推进，没有声学回环录音。

## 自动回归

[run_checks.py](run_checks.py) 的十项检查全部通过，只保留结构化状态，不保存原始诊断输出，详见 [checks.json](checks.json)。

- `go test -race -count=1 ./...`：1,013 项通过、20 项跳过，未使用测试缓存，无失败。
- 歌词及首次播放专项：20 项通过，覆盖默认面板、手动选择、键盘/鼠标滚动、跟随恢复、seek、暂停、切歌、布局变化、歌词清理和纯文本 fallback。
- 原生 UI Tester 覆盖 420×640、960×740、1440×900。断言首句和跳播后的当前句可见，以及浮动/紧凑沉浸页面外层不会被持续拖动；共输出 10 张快照，已目检 420×640、960×740 首次播放及 420 宽沉浸式快照。[420×640 首次播放](ui-tester/first-play-420x640.png) · [960×740 首次播放](ui-tester/first-play-960x740.png) · [结果](ui-tester.json)
- `go vet ./...`、翻译检查、运行时清单和协议检查、完整运行时测试、登录 helper 单元测试及完整性验证、`git diff --check` 均通过。

## 补验结果、剩余问题与覆盖边界

1. **窄窗口实机操作已补验**：通过绿灯菜单和显示器移动命令取得 757×641 紧凑布局，首次播放默认歌词、seek、手动阅读/恢复、外层稳定和沉浸页均已操作验证。[补验报告](retry/README.md)
2. **全局快捷键实体输入**：设置保留 ⇧⌘Space。向前台 Calculator 发送该组合后，ytmgo 的播放状态没有切换。当前证据无法区分按键注入限制与真实快捷键问题，本项没有验收通过，也没有直接归为产品缺陷。[第一次观察](37-global-hotkey-pause.ax.txt) · [稳定后观察](38-global-hotkey-settled.ax.txt)
3. **音频中断根因未确定**：补验记录一次 `audio_stream_failed`；Retry 后持续播放并自动进入下一首，恢复路径通过，发生原因仍未关闭。[诊断元数据](retry/05-audio-status.json)

本轮未覆盖实体触控板斜向手势、网络故障注入、睡眠唤醒、崩溃/强制终止、退出或切换账号、Windows/Linux、长期稳定性或公开公证安装包。若要求这些范围一并通过，当前证据不足以声称完整验收结束。

工具曾出现未就绪控件、异步状态变化、窗口菜单元素失效及无法完成的窗口缩放；通过刷新 AX 状态继续可执行用例，没有将工具错误当作产品崩溃或验收通过。进程诊断只使用 `safe-processes` 的 PID、父进程、状态与可执行路径；未读取 argv、环境内容、认证存储或原始诊断日志。
