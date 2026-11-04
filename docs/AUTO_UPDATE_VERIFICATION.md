# 自动升级验收

## 0.6.4 — 2026-11-04

**PASS（macOS 包与原生启动）**。最低部署目标降至 macOS 13，新增 Intel 包及独立的 `update-darwin-amd64.json`。ARM 与 Intel 的签名、公证、Gatekeeper、随包组件和原生启动均通过；公开 Homebrew ZIP 在两种架构上实际安装、启动通过。独立公开下载验证了两套更新归档的摘要和 Ed25519 签名，以及全部 Mach-O 的架构与最低版本。

公开 0.6.3 经生产 feed 升级至 0.6.4 后，应用文件、权限及符号链接与独立公开 ZIP 一致，签名及公证票据有效。[升级回执](audits/2026-11-04-macos-compat-0.6.4/public-upgrade-receipt.json) · [完整验收与边界](audits/2026-11-04-macos-compat-0.6.4/README.md)。macOS 13/14 实机和 Intel 真实账号播放 E2E 尚未验证。


## 0.6.3 — 2026-11-04

Windows x64 和 Linux x64 已补充首批原生安装包及各自的 Ed25519 签名更新源。公开下载后的归档签名、平台校验清单及稳定 feed 与版本 feed 的一致性检查通过。两个平台尚无上一正式安装包基线，本轮不宣称完成了真实跨版本更新。[跨平台安装包回执](audits/2026-11-04-desktop-release-0.6.3/public-verification.json)

**PASS**。独立 macOS runner 从公开的 0.6.2 ZIP 出发，通过生产 feed 和维护命令完成 0.6.3 升级；更新后的文件、权限、符号链接与公开 0.6.3 ZIP 一致，签名、公证票据及 Gatekeeper 检查通过。该验证使用隔离应用副本，没有初始化账号存储。[公开包升级回执](audits/2026-11-04-release-0.6.3/public-upgrade-receipt.json)

本机通过新增加的 Homebrew cask 安装正式 0.6.3，全部 725 项应用条目与公开 ZIP 一致。Premium、曲库、25 首队列、29.765 秒暂停断点及静音状态恢复。此次本机安装走 Homebrew，未重复操作原生更新窗口；也未声称完成扬声器听感、实体全局快捷键或睡眠唤醒验收。[完整记录](audits/2026-11-04-release-0.6.3/README.md) · [Homebrew 安装](HOMEBREW.md)

## 0.6.2 — 2026-11-04

**PASS**。本机从公开的 0.6.1 通过 **Check for Updates… → Install Update → Relaunch Now** 升级至 0.6.2。自动重启后，设置与更新窗口均显示 0.6.2。安装包与独立下载的正式 ZIP 一致，Apple 签名、公证票据和 Gatekeeper 检查通过。[发布与本机验收记录](audits/2026-11-04-release-0.6.2/README.md)

Premium、个人曲库和封面在升级后可用。播放器重启时处于空闲状态，手动恢复升级前的曲目和进度后可正常播放。远端还从公开 0.6.1 ZIP 单独验证了生产更新源、安装及最终应用包。[公开包升级回执](audits/2026-11-04-release-0.6.2/public-upgrade-receipt.json)

## 0.6.1 — 2026-11-03

**PASS**：已公开发布 [0.6.1](https://github.com/mreasonyang/ytmgo/releases/tag/v0.6.1)，完成本机原生更新窗口升级及独立的公开 0.6.0 → 0.6.1 包升级。

- [构建与发布](https://github.com/mreasonyang/ytmgo/actions/runs/41596986992)完成 Developer ID 签名、Apple 公证、更新签名和全部六项资产发布，源码及 tag 为 `a3193c5d1a53fb1445a3edff9516fc98841821d0`。
- 本机已安装的签名 E2E 0.6.0 构建，经 **Check for Updates → Install Update → Relaunch Now** 自动升级并重启至 0.6.1。再次检查显示已是最新版；Premium、头像、曲库及语言、外观、音量和快捷键设置恢复。安装后全部文件、权限及符号链接与公开 ZIP 一致，Apple 签名、公证票据及 Gatekeeper 通过。
- [独立公开包升级](https://github.com/mreasonyang/ytmgo/actions/runs/41598046551)另从正式 0.6.0 ZIP 出发，在隔离 bundle 中通过生产 feed 完成升级及同样的完整性校验。
- 升级后的真实账号播放复测确认 Antidote 与 Random Access Memories 的曲目封面、歌手、当前播放和队列展示恢复。

基线区别、范围和结构化回执见 [0.6.1 发布与应用内升级验收](audits/2026-11-03-release-0.6.1/README.md)。本次判定升级链路通过，不替代睡眠唤醒、其他平台或长时间运行验收。

## 0.6.0 — 2026-11-02

**PASS**：已公开发布 [0.6.0](https://github.com/mreasonyang/ytmgo/releases/tag/v0.6.0)，生产 feed 已指向 0.6.0，并从真实公开的 0.5.1 包完成升级。

- [构建与发布](https://github.com/mreasonyang/ytmgo/actions/runs/41380657462)通过 Go、runtime/login、翻译与打包检查，完成 App/DMG Developer ID 签名、Apple 公证、staple、Gatekeeper、更新包签名及远端资产摘要比对，发布全部六项资产。tag 与 `build.json` 均指向 `c15f02a005f25f97d3dd09e355119dad2a3c58cc`。
- [公开包升级验收](https://github.com/mreasonyang/ytmgo/actions/runs/41381505029)在 macOS 27.0.1 / arm64 通过。升级后的版本命令和 bundle 版本均为 0.6.0，Apple 签名、stapled ticket、Gatekeeper 均通过；全部文件、权限及符号链接与独立下载的公开 0.6.0 ZIP 一致。回执是该运行的 `ytmgo-public-upgrade-receipt-3` artifact。
- 本机匿名下载的公开元数据通过核对，稳定 feed 与版本专属 feed 一致；GitHub 返回的全部资产摘要与公开校验清单一致。

产物与结构化回执见 [0.6.0 发版记录](audits/2026-11-02-release-0.6.0/README.md)。升级验收使用隔离的公开 bundle 与维护命令，没有初始化账号存储或替换本机安装版；本次没有重复原生更新窗口、真实账号或播放验收。

| 产物 | 来源或 SHA-256 |
| --- | --- |
| 源码 | `c15f02a005f25f97d3dd09e355119dad2a3c58cc` |
| DMG | `2e43507475001690190be7bc34ff6348cc52eaa15db1c8fd181e1b27714ea5ab` |
| ZIP | `58e29e2eed4819d5ca19faed48bdf37468624ea631fdb6adf0ea0ae87f13234f` |
| 更新 archive | `f245f16dd8b2796c4fbd93b1f1043c3f0a40a5403036b1beae12a93ee57ad5b7` |

## 0.5.1 — 2026-10-30

**PASS**：已公开发布 [0.5.1](https://github.com/mreasonyang/ytmgo/releases/tag/v0.5.1)，生产 feed 已指向 0.5.1，并从真实公开的 0.5.0 包完成升级。

- [构建与发布](https://github.com/mreasonyang/ytmgo/actions/runs/41005003646)通过完整源码、runtime/login、翻译及构建检查，完成 App/DMG Developer ID 签名、Apple 公证、staple、Gatekeeper、更新包签名及远端资产摘要比对。
- [公开包升级验收](https://github.com/mreasonyang/ytmgo/actions/runs/41005817902)通过。升级后的版本命令和 bundle 版本均为 0.5.1，Apple 签名、票据和 Gatekeeper 均通过，全部文件、权限及符号链接与独立下载的公开 0.5.1 ZIP 一致。回执是该运行的 `ytmgo-public-upgrade-receipt-2` artifact。
- 本机匿名下载的 0.5.1 DMG 通过公开 SHA-256、签名、公证票据、Gatekeeper 和镜像完整性检查。生产 feed 版本及 `build.json` 源码提交与本次发布一致。

该升级检查使用隔离的公开 bundle 与维护命令，没有初始化账号存储，也没有替换维护者 `/Applications` 中的应用。原生更新窗口点击、真实账号登录和播放验收属于其他检查，本次发布验证未重复这些场景。

### 0.5.1 产物身份

| 产物 | 来源或 SHA-256 |
| --- | --- |
| 源码 | `52790e0161c534f391eb620bf956bc60649b1df0` |
| DMG | `9dcadfef7bc16bcce3360a4a1792a488b5822b3cb835ffaa01f4162906b34dac` |
| ZIP | `f253c2c0fbd06fae114307532efe3098bfce7e5da3e4d21eb7f0b959a64ecf1f` |
| 更新 archive | `74fc5eb06ce1902876affa5ffbf09707ba2951f58798b4998e1c977e7f8ec799` |

## 0.5.0 — 历史验收

日期：2026-10-29。目标：macOS 27、Apple Silicon。

### 结果

**PASS**：已公开发布 [0.5.0](https://github.com/mreasonyang/ytmgo/releases/tag/v0.5.0)，并从真实公开发布的 [0.5.0-rc.1](https://github.com/mreasonyang/ytmgo/releases/tag/v0.5.0-rc.1) 通过生产更新 feed 完成原地升级。

- [正式版构建与发布](https://github.com/mreasonyang/ytmgo/actions/runs/40838950530)：源码测试、runtime/login 检查、构建、Developer ID 签名、App 和 DMG 公证、staple、Gatekeeper、更新包 Ed25519 签名、公钥独立验签、远端资产摘要比对及公开发布全部通过。
- [公开包升级验收](https://github.com/mreasonyang/ytmgo/actions/runs/40840195521)：匿名下载基线 ZIP，核验公开 SHA256SUMS、签名、票据和 Gatekeeper；基线客户端从生产 feed 发现 0.5.0，下载并校验更新签名，退出后由 MyGo helper 替换 bundle；再次核验版本、签名、票据和 Gatekeeper，并与独立下载的正式版 ZIP 比较所有文件、权限及符号链接，完全一致。详细回执见该运行的 `ytmgo-public-upgrade-receipt-1` artifact 与 job summary。
- 本地 Go 测试、`go test -race ./...`、`go vet ./...`、翻译检查、22 项 Python 打包/发布测试和 actionlint 通过。
- 额外的合成协议验证中，真实 MyGo updater 拒绝被篡改的压缩包，原应用保留；正确 Ed25519/SHA-256 签名的包通过并在退出后替换。它与上述公开包升级验收分别记录。
- 本机 macOS 27.0.1 / arm64 匿名下载的基线 ZIP 摘要、Apple 签名、stapled ticket、Gatekeeper 和版本命令也通过。

升级验收使用应用内的维护命令，调用与原生更新窗口相同的 `mygo.Updater.Check` 和 `Update.Install`。该次检查未点击原生更新窗口或验证 GUI 重启，未启动账号、播放逻辑，也未替换维护者 `/Applications` 中的应用。

### 产物身份

| 产物 | 来源或 SHA-256 |
| --- | --- |
| 0.5.0-rc.1 源码 | `b769cc195ba1e5b7daad777f6791cb0b955960ec` |
| 0.5.0 源码 | `eb8e462a107468690ea99f7e577f060284747457` |
| 0.5.0 DMG | `a8df3b0a57eb28a58a35cfce43932fd22d1686c6a6140e20a4755a4eae7e9f43` |
| 0.5.0 ZIP | `3bdf7a7bc01e6cd2a6a32b495b4d29a059b95bb895d4e447a879f5b929d6d681` |
| 0.5.0 更新 archive | `6d26c5aad7fb7daa116f4e5b80e84a30d063c58e073c2ea2c622a4461f53fa12` |

稳定更新地址：`https://github.com/mreasonyang/ytmgo/releases/latest/download/update-darwin-arm64.json`。本机匿名读取已确认它指向 0.5.0。

预发行版的首次运行在已完成签名、公证和资产上传后，因使用 REST tag endpoint 查询 draft 返回 404，停在发布门禁。修复改为查询 draft 的 release ID；已上传资产与 CI 校验清单、GitHub 摘要和 Ed25519 签名重新核验后才公开预发行版。正式版通过修复后的完整工作流，无需这个恢复步骤。

### 使用与后续发布

0.4.0 没有更新器，需手动安装一次 0.5.0。此后启动自动检查，菜单 **ytmgo → 检查更新** 可手动检查；更新窗口可选择自动下载，下载后重启安装。

后续发布和签名密钥维护见 [macOS Actions 指南](MACOS_ACTIONS.md)。公开包升级检查可以在 [macOS public update check](https://github.com/mreasonyang/ytmgo/actions/workflows/macos-update-check.yml) 手动运行，填写已发布的 `from_version` 和当前稳定 feed 的 `to_version`。该工作流不绑定签名证书或更新私钥。
