# 窄窗口实机补验

2026-11-04（Asia/Shanghai），应用户“重试”继续验收。**757×641 紧凑窗口的歌词主流程通过。** 420×640 仍只有初轮原生 UI Tester 证据；本页不将其改称为 420px 实机测试。

通过窗口绿灯菜单和 Window 菜单，先在外接屏获得 961×641 短窗口，再移至内建屏并平铺，实际进入 `width <= 760` 的紧凑布局。两种尺寸分别由 1922×1282、1514×1282 截图核对。结束后已移回 DELL 显示器并恢复原宽窗口。

| 场景 | 结果与证据 |
| --- | --- |
| 紧凑窗口冷启动及首次播放 | 冷启动时保持暂停；首次播放《People Help the People》后自动打开 Lyrics，当前句可直接看见，没有常驻 Following playback 按钮。[冷启动 AX](10-compact-cold-start.ax.txt) · [首次播放截图](11-compact-first-play.jpg) |
| 暂停与远距离跳播 | 暂停后跳到 177.370729557 秒，保持暂停并定位到当前 cue；其内容在面板内完整可见。[跳播截图](12-compact-seek.jpg) |
| 外层滚动稳定 | 手动向上滚回封面并继续播放，177.501396223→203.058729557 秒期间封面保持原位；歌词跟随没有将外层拉回。两张截图的静态封面区域像素差已作断言。[开始](13-compact-cover-reading.jpg) · [稍后](14-compact-cover-later.jpg) |
| 手动阅读与恢复 | 回到歌词后手动滚动，才出现 Follow playback；点击恢复后按钮消失并回到当前句。[手动状态](15-compact-manual.jpg) · [恢复状态](16-compact-resumed.jpg) |
| 紧凑沉浸式页面 | 可进入沉浸页并滚动到当前歌词，当前高亮句完整可读。[截图](17-compact-immersive.jpg) |

补验另外记录了一次真实播放中断：`Can't Help Falling In Love` 播放到约 168.28 秒后显示无法播放和 Retry。白名单诊断记录为 `audio_stream_failed`，缺少更细的原因，**根因尚未确定**。点击 Retry 后恢复到 192.31 秒并继续播放，随后正常自动进入下一首。[故障现场](04-playback-error.jpg) · [仅固定字段的诊断](05-audio-status.json) · [恢复](06-after-retry.ax.txt) · [下一首](07-short-revealed.ax.txt)

全局 ⇧⌘Space 再次使用 250ms 按键持续时间重试，播放状态仍没有切换。本项仍未验收通过，当前证据不能区分工具注入限制和真实快捷键问题，需实体键盘复核。[重试 AX](18-global-hotkey-foreground.ax.txt)

补验始终使用同一安装二进制 `cd91528fd113aeec09ffa522372a8f5c3bb7c168349e818c47dbfe874a2631c9`，178 项安装文件/链接与初轮构建仍完全一致，没有修改产品源码或重建应用。期间工作区的 `acceptance_probe_test.go`、`internal/player/player.go`、`internal/player/player_test.go`、`native_settings.go` 出现额外变化；这些变化未进入本轮安装产物，初轮自动回归数字只对应当时绑定的源码，不能套用到当前新增变化。

最终原歌曲、90 首队列、index=8、162.466666666 秒、70% 音量均已恢复，Premium 已恢复，应用留在原外接屏宽窗口并暂停。[最终截图](21-final-restored.jpg)

[verify_evidence.py](verify_evidence.py) 断言安装产物、窗口尺寸、播放推进、恢复按钮、外层稳定、故障恢复和最终播放文件，结果见 [result.json](result.json)。本次没有重复运行针对未进入该二进制的当前工作区测试。

歌词主流程的窄窗口缺口已补齐；全局快捷键及一次音频中断的原因仍未关闭，因此完整应用验收仍不能宣称全部通过。
