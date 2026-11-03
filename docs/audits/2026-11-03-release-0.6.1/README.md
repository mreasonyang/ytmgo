# 0.6.1 发布与应用内升级验收

日期：2026-11-03。平台：macOS / Apple Silicon。

**PASS：远程签名、公证和发布完成；本机通过原生更新窗口从 0.6.0 升级至 0.6.1，自动重启并恢复账号和曲库。**

## 发布与产物身份

- [公开版本 v0.6.1](https://github.com/mreasonyang/ytmgo/releases/tag/v0.6.1)，源码及 tag 为 `a3193c5d1a53fb1445a3edff9516fc98841821d0`。
- [远程构建与发布](https://github.com/mreasonyang/ytmgo/actions/runs/41596986992)通过 Go、runtime、login、vet、翻译和打包检查，完成 Developer ID 签名、Apple 公证、staple 及正式发布；六项公开资产齐全。
- 独立下载的公开资产摘要与校验清单一致，更新 archive 的 Ed25519 签名通过，生产 feed 与版本专属 feed 一致。
- ZIP SHA-256：`886d851713ae631c5f4699d077ede7aa833d5654533b9ce24e46ca8a59c107fd`。
- 安装后主程序 SHA-256：`fd078a3d5f0719fdcbff7eec037fdf1d388384ed9758fef5c776be37f5b46113`。

## 本机原生更新窗口

基线是 `/Applications/ytmgo.app` 中已安装的 Developer ID 签名本地 E2E 0.6.0 构建，并非公开 0.6.0 ZIP。正式公开包之间的升级另行验证如下。

1. 发布前点击 **ytmgo → Check for Updates…**，显示 0.6.0 已是最新版。
2. 发布后再次点击，显示当前版本 0.6.0、可用版本 0.6.1。
3. 点击 **Install Update**，观察下载进度及 **Ready to Relaunch**。
4. 点击 **Relaunch Now**；在再次获取应用 UI 前，已观察到新主进程自动启动。
5. 设置页显示 **Version 0.6.1**；再次检查更新显示 **ytmgo 0.6.1 is currently the newest version available.**

整个安装由原生更新器完成。安装后的全部文件字节、权限及符号链接与独立下载的公开 ZIP 一致；Apple 签名、stapled ticket 和 Gatekeeper 均通过。Premium 状态、头像、保存的曲库恢复，English、Dark、音量 0.7、Command+Shift+Space 配置保留，更新窗口的未来自动更新选项维持原来的未勾选状态。

## 专辑元数据修复复测

根因是上游专辑曲目省略公共封面和部分歌手信息，旧解析器没有从专辑头部补齐。Antidote 的 13 行均省略封面和歌手字段；Random Access Memories 的 13 行省略封面，但保留歌手链接。修复仅为明确的专辑主曲目列表补齐缺失字段，保留曲目自己的作者和合作艺人；分页只继承所属曲目列表的专辑上下文。

联网回归中，两张专辑均为 13/13 有封面、13/13 有歌手。升级后的安装版确认曲目列表、当前播放、Now Playing 和可见队列封面正常，13 首曲目的歌手标签完整，Random Access Memories 的合作艺人保留。Antidote 的 1991 播放推进至 52.069 秒，Random Access Memories 的 Give Life Back to Music 推进至 130.725 秒；完成后暂停。

修复源码的完整 `go test -race ./...` 为 972 通过、20 跳过，运行期间源码指纹未变；catalog race 为 213 通过、4 跳过。版本递增后的远程构建重新通过源码检查。

## 公开正式包升级

[独立远程升级验收](https://github.com/mreasonyang/ytmgo/actions/runs/41598046551)从正式公开的 0.6.0 ZIP 出发，使用生产 feed 和维护命令完成 0.6.1 原地升级。升级后版本、Apple 签名、stapled ticket、Gatekeeper 及与公开 ZIP 的完整 bundle 比较均通过。该检查使用隔离 bundle，没有初始化账号存储；它与上述本机原生 UI 验收分别提供证据。

## 范围及回执

本记录判定发布、升级链路和专辑元数据修复通过。先前完整 E2E 中的视频首次启动偶发失败、全局快捷键实际系统触发与睡眠唤醒验证，不因本次升级通过而关闭；本次也不覆盖其他平台或长时间运行。

- [原生升级与安装完整性](native-upgrade.json)
- [更新窗口检查与点击结果](updater-ui.json)
- [账号、设置与播放复测](post-upgrade-ui.json)
- [公开资产独立验证](public-verification.json)
- [远程构建结果](package-workflow.json)
- [公开包升级回执](public-upgrade-receipt.json)及[工作流结果](public-upgrade-workflow.json)
- [完整 race](final-full-race.json)及[catalog race](final-catalog-race.json)
