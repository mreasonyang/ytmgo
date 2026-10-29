# macOS Actions 打包与签名

工作流：[macOS package](https://github.com/mreasonyang/ytmgo/actions/workflows/macos-package.yml)。
在 **Run workflow** 中选择 `main` 和模式。当前只打包 **Apple Silicon / arm64**，应用最低系统要求仍是 **macOS 27**。使用 GitHub `xcode-27` runner；这个镜像目前属于 public preview，镜像可用性与应用兼容性分别判断。

| 模式 | 需要的配置 | 结果 |
| --- | --- | --- |
| `unsigned` | 无证书 | ad-hoc App ZIP、未签名 DMG，供构建测试 |
| `developer-id` | Developer ID `.p12`、导出密码、Team ID | Developer ID 签名 App / DMG，未公证 |
| `notarized` | 上述配置及 App Store Connect team API key | 签名并公证 App / DMG，附带 stapled tickets |

所有模式先执行 `make check` 和 `make build`。成功后在对应运行页面的 **Artifacts** 下载 DMG、保留 App bundle 的 ZIP、`SHA256SUMS` 与 `build.json`，保留 14 天。`build.json` 记录版本、提交、模式和最低系统要求。工作流不创建 GitHub Release；公开仓库的运行记录和构建 artifacts 对有相应 GitHub 访问权限的人可见。

## 1. 导出 Developer ID Application 证书

在 Mac 打开 **钥匙串访问 → 登录 → 我的证书**，找到 **Developer ID Application**。展开确认其下包含私钥，选择该证书与私钥，导出为密码保护的 `.p12`。不要选择 **Apple Development** 或 **Developer ID Installer**。单独 `.cer` 不含签名所需私钥。

若尚未创建证书，在 [Apple Developer Certificates](https://developer.apple.com/account/resources/certificates/list) 创建 **Developer ID Application**，按网页要求在本机生成 CSR、安装证书，再从钥匙串导出。需要有效的 Apple Developer Program 会员及相应证书权限。

`.p12` 放在仓库外自己的私有目录。不要将它、私钥或导出密码放入聊天、提交、日志或截图。

## 2. 配置 GitHub environment

环境：[macos-release](https://github.com/mreasonyang/ytmgo/settings/environments)。部署分支策略只允许 `main`，工作流也只接受 `main`。普通 pull request 的登录测试没有签名 secrets。

在该环境配置以下 **Environment secrets**：

| 名称 | 内容 |
| --- | --- |
| `MACOS_CERTIFICATE_P12_BASE64` | 完整 `.p12` 文件的 base64 |
| `MACOS_CERTIFICATE_PASSWORD` | `.p12` 导出密码 |
| `APPLE_API_KEY_P8_BASE64` | 公证用 team API key `.p8` 文件的 base64 |
| `APPLE_API_KEY_ID` | 该 API key 的 Key ID |
| `APPLE_API_ISSUER_ID` | 该 team API key 的 Issuer ID |

环境 **Variables** 的 `APPLE_TEAM_ID` 必须与证书括号中的 10 位 Team ID 相同。签名脚本会拒绝错误团队或多个匹配身份。

推荐在你自己的交互终端运行配置助手；它会询问文件路径，隐藏输入密码，再通过 `gh` 的 stdin 直接上传 encrypted secrets，不产生 base64 中间文件，也不把秘密值放入命令行参数。

```sh
cd /Users/easonyang/Documents/projects/ytmgo
python3 scripts/release/configure_secrets.py signing
```

助手使用当前 `gh` 登录，固定写入 `mreasonyang/ytmgo` 的 `macos-release`。如果尚未登录，先运行 `gh auth login`。可在 GitHub 环境页面或 `gh secret list --repo mreasonyang/ytmgo --env macos-release` 检查名称；不要读取或输出值。

## 3. 配置 Apple 公证

到 [App Store Connect → Users and Access → Integrations → App Store Connect API](https://appstoreconnect.apple.com/access/integrations/api)，创建有公证权限的 **Team API Key**。使用 Developer 或更高的适用角色；若账号尚无 API access，先由 Account Holder 申请。下载 `.p8`，记下 Key ID 和 Issuer ID。私钥只可下载一次，应存入仓库外自己的私有目录。

这里使用带 Issuer ID 的 **team key**，不使用 individual key；不需要把 Apple ID 登录密码或二次验证码交给 GitHub。

```sh
python3 scripts/release/configure_secrets.py notarization
```

## 4. 运行与验证

首次可以选择 `unsigned` 验证云端依赖与构建，再配置证书选择 `developer-id`，最后选择 `notarized`。命令行也可启动：

```sh
gh workflow run macos-package.yml --repo mreasonyang/ytmgo --ref main -f mode=notarized
```

签名脚本给每个 Mach-O、登录 helper app、主 App 依次签名，启用 Hardened Runtime 和安全时间戳。只有 Node 获得 JIT 相关 entitlements。签名改变二进制字节后，重算包内 runtime / login 清单，再密封主 App。签名后执行 Node 和 FFmpeg 冒烟验证。

公证模式先提交 App ZIP，Apple 返回 `Accepted` 后给 App staple ticket；再制作、签名、公证 DMG 并 staple。最后检查签名、ticket 与 Gatekeeper。任一步失败都不会上传成功产物，**不会降级生成 unsigned 成功包**。Actions 成功不代表真实账号登录、播放或所有支持设备验收完成。

证书只在签名步骤可用，导入临时 keychain；`.p12`、`.p8`、临时 keychain 在结束时清理，另有 `always()` 清理步骤。外部签名工具的原始输出不会进入日志；只报告固定失败阶段和公证 submission ID。Apple 的 keychain 工具接口需要通过子进程参数接收密码，这些参数不打印、不写入证据；不要对 runner 执行 argv/environment 诊断，也不要开启 shell tracing 或 Actions step debug 来排错。

缺少 setting 时按报错名称补齐。`Certificate import failed` 检查 `.p12` 是否含私钥、密码是否正确；`Expected exactly one valid Developer ID…` 检查证书有效期、类型和 Team ID。公证失败时先核对 team key 的权限、Key ID、Issuer ID；Apple 延迟时可重新运行，证书与 Apple 公证服务结果需分别验证。

参考：[Apple notarization](https://developer.apple.com/documentation/security/notarizing-macos-software-before-distribution)、[notarytool API-key authentication](https://developer.apple.com/documentation/technotes/tn3147-migrating-to-the-latest-notarization-tool)、[GitHub Xcode 27 runner image](https://github.com/actions/runner-images/blob/main/images/macos/xcode-27-arm64-Readme.md)。
