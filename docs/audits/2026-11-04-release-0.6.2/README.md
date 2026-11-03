# 0.6.2 发布与应用内升级验收

**PASS**。2026-11-04，本机 Apple Silicon Mac 经原生更新窗口从公开的 0.6.1 升级到 0.6.2。更新后的正式版恢复了 Premium 会话和个人曲库，成功播放了升级前的同一首歌。[正式版](https://github.com/mreasonyang/ytmgo/releases/tag/v0.6.2) · [发布提交](https://github.com/mreasonyang/ytmgo/commit/f2c50e8b0df3fb7a743fde9571070251a26d0b05)

| 核验项 | 结果 |
| --- | --- |
| 本机升级起点 | 安装的 0.6.1 与公开 ZIP 中的 202 个条目一致。发布前手动检查，更新窗口显示 0.6.1 已是最新版。 |
| 更新操作 | 0.6.1 从生产更新源发现 0.6.2。点击 Install Update 后看到下载进度和 Ready to Relaunch。点击 Relaunch Now，更新器在原位置替换应用并自动启动了新进程。 |
| 本机升级终点 | 设置页显示 Version 0.6.2，再次检查更新显示已是最新版。安装后的所有文件内容、权限和符号链接均与独立下载的公开 0.6.2 ZIP 一致，共 202 个条目。 |
| 分发验证 | 6 项公开资产的 SHA-256 与 GitHub 摘要一致。更新包通过 Ed25519 验签，稳定 feed 与版本专属 feed 相同。App 和 DMG 的 Developer ID 签名、公证票据和 Gatekeeper 检查通过。 |
| 干净安装副本 | 独立 macOS runner 从公开 0.6.1 ZIP 读取同一生产 feed，通过维护命令升级到 0.6.2，最终应用包与公开 ZIP 一致。 |

[本机更新窗口](updater-ui.json) · [安装包完整性](installed-verification.json) · [公开产物校验](public-verification.json) · [签名和发布工作流](https://github.com/mreasonyang/ytmgo/actions/runs/41674826271) · [独立升级工作流](https://github.com/mreasonyang/ytmgo/actions/runs/41676926200) · [升级回执](public-upgrade-receipt.json)

**封面缓存。** 正式版显示了个性化首页、曲库侧栏的 Liked Music 和 Discover Mix 封面，以及专辑曲目封面。本机候选测试已留下公开图片缓存。升级后有 25 个已有文件的读取时间更新，文件大小、inode 和到期时间均相同。此次采样共有 39 个文件、1,562,096 字节。实现上限为 512 个文件、64 MiB，缓存有效期最长 24 小时。

**Premium 播放。** 升级前正在播放。重启后账号自动恢复，播放器处于空闲状态。重新从原专辑打开同一首歌，播放进度持续前进。将播放条移回升级前记录的约 3 分 51 秒处后，确认进度继续推进至 3 分 57 秒，音量保持 70%。

**回归范围。** 本地全量 Go 竞态测试有 981 项通过、27 项跳过，10 个包通过。Go vet、四种语言的 239 条翻译及 23 项发布脚本测试通过。播放运行时 CI 的 macOS、Windows、Linux 全部通过。另一个登录组件 CI 的 macOS 作业通过，Windows 和 Linux 作业各有 6 个失败的 Go 用例，各平台的失败用例名称与 0.6.1 完全相同。本次发布及原生升级验收覆盖 macOS arm64。[回归明细](source-checks.json) · [播放 CI](https://github.com/mreasonyang/ytmgo/actions/runs/41674807334) · [登录 CI](https://github.com/mreasonyang/ytmgo/actions/runs/41674807423)
