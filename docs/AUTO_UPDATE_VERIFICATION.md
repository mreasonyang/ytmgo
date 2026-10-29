# 0.5.0 自动升级验收

日期：2026-10-29。目标：macOS 27、Apple Silicon。

## 结果

**PASS**：已公开发布 [0.5.0](https://github.com/mreasonyang/ytmgo/releases/tag/v0.5.0)，并从真实公开发布的 [0.5.0-rc.1](https://github.com/mreasonyang/ytmgo/releases/tag/v0.5.0-rc.1) 通过生产更新 feed 完成原地升级。

- [正式版构建与发布](https://github.com/mreasonyang/ytmgo/actions/runs/40838950530)：源码测试、runtime/login 检查、构建、Developer ID 签名、App 和 DMG 公证、staple、Gatekeeper、更新包 Ed25519 签名、公钥独立验签、远端资产摘要比对及公开发布全部通过。
- [公开包升级验收](https://github.com/mreasonyang/ytmgo/actions/runs/40840195521)：匿名下载基线 ZIP，核验公开 SHA256SUMS、签名、票据和 Gatekeeper；基线客户端从生产 feed 发现 0.5.0，下载并校验更新签名，退出后由 MyGo helper 替换 bundle；再次核验版本、签名、票据和 Gatekeeper，并与独立下载的正式版 ZIP 比较所有文件、权限及符号链接，完全一致。详细回执见该运行的 `ytmgo-public-upgrade-receipt-1` artifact 与 job summary。
- 本地 Go 测试、`go test -race ./...`、`go vet ./...`、翻译检查、22 项 Python 打包/发布测试和 actionlint 通过。
- 额外的合成协议验证中，真实 MyGo updater 拒绝被篡改的压缩包，原应用保留；正确 Ed25519/SHA-256 签名的包通过并在退出后替换。它与上述公开包升级验收分别记录。
- 本机 macOS 27.0.1 / arm64 匿名下载的基线 ZIP 摘要、Apple 签名、stapled ticket、Gatekeeper 和版本命令也通过。

升级验收使用应用内的维护命令，调用与原生更新窗口相同的 `mygo.Updater.Check` 和 `Update.Install`。该次检查未点击原生更新窗口或验证 GUI 重启，未启动账号、播放逻辑，也未替换维护者 `/Applications` 中的应用。

## 产物身份

| 产物 | 来源或 SHA-256 |
| --- | --- |
| 0.5.0-rc.1 源码 | `b769cc195ba1e5b7daad777f6791cb0b955960ec` |
| 0.5.0 源码 | `eb8e462a107468690ea99f7e577f060284747457` |
| 0.5.0 DMG | `a8df3b0a57eb28a58a35cfce43932fd22d1686c6a6140e20a4755a4eae7e9f43` |
| 0.5.0 ZIP | `3bdf7a7bc01e6cd2a6a32b495b4d29a059b95bb895d4e447a879f5b929d6d681` |
| 0.5.0 更新 archive | `6d26c5aad7fb7daa116f4e5b80e84a30d063c58e073c2ea2c622a4461f53fa12` |

稳定更新地址：`https://github.com/mreasonyang/ytmgo/releases/latest/download/update-darwin-arm64.json`。本机匿名读取已确认它指向 0.5.0。

预发行版的首次运行在已完成签名、公证和资产上传后，因使用 REST tag endpoint 查询 draft 返回 404，停在发布门禁。修复改为查询 draft 的 release ID；已上传资产与 CI 校验清单、GitHub 摘要和 Ed25519 签名重新核验后才公开预发行版。正式版通过修复后的完整工作流，无需这个恢复步骤。

## 使用与后续发布

0.4.0 没有更新器，需手动安装一次 0.5.0。此后启动自动检查，菜单 **ytmgo → 检查更新** 可手动检查；更新窗口可选择自动下载，下载后重启安装。

后续发布和签名密钥维护见 [macOS Actions 指南](MACOS_ACTIONS.md)。公开包升级检查可以在 [macOS public update check](https://github.com/mreasonyang/ytmgo/actions/workflows/macos-update-check.yml) 手动运行，填写已发布的 `from_version` 和当前稳定 feed 的 `to_version`。该工作流不绑定签名证书或更新私钥。
