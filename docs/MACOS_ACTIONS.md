# macOS Actions 打包与签名

工作流：[macOS package](https://github.com/mreasonyang/ytmgo/actions/workflows/macos-package.yml)。
在 **Run workflow** 中选择 `main` 和模式。当前只打包 **Apple Silicon / arm64**，应用最低系统要求仍是 **macOS 27**。使用 GitHub `xcode-27` runner；这个镜像目前属于 public preview，镜像可用性与应用兼容性分别判断。

| 模式 | 需要的配置 | 结果 |
| --- | --- | --- |
| `unsigned` | 无证书 | ad-hoc App ZIP、未签名 DMG，供构建测试 |
| `developer-id` | Developer ID `.p12`、导出密码、Team ID | Developer ID 签名 App / DMG，未公证 |
| `notarized` | 上述配置及 Apple ID、应用专用密码 | 签名并公证 App / DMG，附带 stapled tickets |

所有模式先逐步执行与 `make check` 相同的检查，再执行 `make build-app` 构建完整 App，最终在签名步骤制作 DMG，避免重复生成一个未使用的 ad-hoc DMG。各检查分开显示，失败时可直接定位命令，无需提取原始诊断日志。成功后在对应运行页面的 **Artifacts** 下载 DMG、保留 App bundle 的 ZIP、`SHA256SUMS` 与 `build.json`，保留 14 天。`build.json` 记录版本、提交、模式和最低系统要求。勾选 `publish_release` 时，仅允许 `notarized` 模式：先创建 draft，上传并核验全部资产的 SHA-256 后再公开发布。发布版本不可覆盖；`X.Y.Z-rc.N` 发布为预发行版，不改变稳定更新 feed，普通 `X.Y.Z` 设为最新稳定版。`release_version` 留空时使用 `mygo.json` 的版本。公开 Release 的 DMG、ZIP 和更新包无需 GitHub 登录即可下载。

## 1. 导出 Developer ID Application 证书

在 Mac 打开 **钥匙串访问 → 登录 → 我的证书**，找到 **Developer ID Application**。展开确认其下包含私钥，选择该证书与私钥，导出为密码保护的 `.p12`。不要选择 **Apple Development** 或 **Developer ID Installer**。单独 `.cer` 不含签名所需私钥。

若尚未创建证书，在 [Apple Developer Certificates](https://developer.apple.com/account/resources/certificates/list) 创建 **Developer ID Application**，按网页要求在本机生成 CSR、安装证书，再从钥匙串导出。需要有效的 Apple Developer Program 会员及相应证书权限。

`.p12` 放在仓库外自己的私有目录。不要将它、私钥或导出密码放入聊天、提交、日志或截图。

## 2. 配置 GitHub environment

环境：[macos-release](https://github.com/mreasonyang/ytmgo/settings/environments)。部署分支策略只允许 `main`，工作流也只接受 `main`。普通 pull request 的登录测试没有签名 secrets。

可以在仓库 [Actions secrets](https://github.com/mreasonyang/ytmgo/settings/secrets/actions) 配置以下 **Repository secrets**，或使用环境 secrets（同名时环境配置优先）。当前仓库使用 Repository secrets：

| 名称 | 内容 |
| --- | --- |
| `MACOS_CERTIFICATE_P12_BASE64` | 完整 `.p12` 文件的 base64 |
| `MACOS_CERTIFICATE_PASSWORD` | `.p12` 导出密码 |
| `APPLE_ID` | 属于该开发者团队的 Apple Account 登录邮箱 |
| `APPLE_APP_SPECIFIC_PASSWORD` | 在 Apple Account 创建的应用专用密码，用于公证 |

环境 **Variables** 的 `APPLE_TEAM_ID` 必须与证书括号中的 10 位 Team ID 相同。签名脚本会拒绝错误团队或多个匹配身份。

推荐在你自己的交互终端运行配置助手；它会询问文件路径，隐藏输入密码，再通过 `gh` 的 stdin 直接上传 encrypted secrets，不产生 base64 中间文件，也不把秘密值放入命令行参数。

```sh
cd /Users/easonyang/Documents/projects/ytmgo
python3 scripts/release/configure_secrets.py signing
```

助手使用当前 `gh` 登录，固定写入 `mreasonyang/ytmgo` 的 `macos-release`。如果尚未登录，先运行 `gh auth login`。可在 GitHub 环境页面或 `gh secret list --repo mreasonyang/ytmgo --env macos-release` 检查名称；不要读取或输出值。

## 3. 配置 Apple 公证

使用 **Apple ID + 应用专用密码 + Team ID** 认证 `notarytool`。到 [Apple Account](https://account.apple.com/) 登录，在 **登录和安全性 → 应用专用密码** 创建一个密码，名称可用 `ytmgo-actions`。Apple Account 需要启用双重认证。步骤见 [Apple 官方说明](https://support.apple.com/en-us/102654)。

在你自己的终端运行以下助手，输入 Apple ID 邮箱和刚生成的应用专用密码。密码会隐藏输入并直接经 stdin 上传到 GitHub encrypted secrets；不用准备 `.p8`、Key ID 或 Issuer ID。

```sh
python3 scripts/release/configure_secrets.py notarization
```

两种密码用途不同：`MACOS_CERTIFICATE_PASSWORD` 是 `.p12` 导出密码，用于导入签名证书；`APPLE_APP_SPECIFIC_PASSWORD` 是 Apple 生成的应用专用密码，用于登录公证服务。不要填 Apple Account 的常用登录密码或双重认证验证码。`APPLE_ID` 必须属于 `APPLE_TEAM_ID` 对应的开发者团队。

## 4. 运行与验证

首次可以选择 `unsigned` 验证云端依赖与构建，再配置证书选择 `developer-id`，最后选择 `notarized`。命令行也可启动：

```sh
gh workflow run macos-package.yml --repo mreasonyang/ytmgo --ref main -f mode=notarized
```

签名脚本给每个 Mach-O、登录 helper app、主 App 依次签名，启用 Hardened Runtime 和安全时间戳。QuickJS 使用解释执行，不需要 Node/V8 的 JIT entitlements。签名改变二进制字节后，重算包内 runtime / login 清单，再密封主 App。签名后执行原生脚本宿主和 FFmpeg 冒烟验证。

公证模式先用应用专用密码创建并验证 runner 临时钥匙串中的 `notarytool` profile，再通过这个 profile 提交 App ZIP。Apple 返回 `Accepted` 后给 App staple ticket；再制作、签名、公证 DMG 并 staple。最后检查签名、ticket 与 Gatekeeper。任一步失败都不会上传成功产物，**不会降级生成 unsigned 成功包**。Actions 成功不代表真实账号登录、播放或所有支持设备验收完成。

证书和公证凭据只在签名步骤可用。证书导入临时 keychain；应用专用密码通过 stdin 进入 `notarytool` 的密码提示，不写入 argv 或明文文件。`.p12`、临时 keychain 及其中的公证 profile 在结束时清理，另有 `always()` 清理步骤。外部签名工具的原始输出不会进入日志；只报告固定失败阶段和公证 submission ID。Apple 的 keychain 管理工具接口仍需要通过子进程参数接收 keychain / `.p12` 密码，这些参数不打印、不写入证据；不要对 runner 执行 argv/environment 诊断，也不要开启 shell tracing 或 Actions step debug 来排错。

缺少 setting 时按报错名称补齐。`Certificate import failed` 检查 `.p12` 是否含私钥、密码是否正确；`Expected exactly one valid Developer ID…` 检查证书有效期、类型和 Team ID。`Notarization credential validation failed` 检查 Apple ID 是否属于该 Team、应用专用密码是否有效；修改或重置 Apple Account 常用登录密码会撤销应用专用密码，需要重新生成和上传。Apple 延迟时可重新运行，证书与 Apple 公证服务结果需分别验证。

参考：[Apple notarization](https://developer.apple.com/documentation/security/notarizing-macos-software-before-distribution)、[notarytool 应用专用密码与钥匙串认证](https://developer.apple.com/documentation/technotes/tn3147-migrating-to-the-latest-notarization-tool)、[GitHub Xcode 27 runner image](https://github.com/actions/runner-images/blob/main/images/macos/xcode-27-arm64-Readme.md)。


## 5. 自动升级发布

应用使用 MyGo 0.7.1 的原生更新窗口。稳定 feed 是：

```text
https://github.com/mreasonyang/ytmgo/releases/latest/download/update-darwin-arm64.json
```

更新器只接受内置公钥对应的 Ed25519 签名。这个密钥与 Apple Developer ID 证书用途不同：Apple 签名和公证用于系统分发信任，更新签名用于确认下载的压缩包来自本项目。不要重新生成密钥来替换已发布版本的公钥，否则已有安装将不能接收更新。

`MYGO_UPDATER_PRIVATE_KEY` 已通过 stdin 配置为本仓库 Actions secret，公钥在 `mygo.json`。私钥备份在维护者机器的仓库外目录 `~/.config/ytmgo/update-signing/mygo-update.key`（0600）；应另行保存在自己的安全备份中，不提交、不输出、不放入截图。它只在签名步骤绑定，通过 stdin 传给提前编译的打包助手，其他子进程会移除该变量。

工作流先给最终 App 完成签名、公证和 staple，再生成 `ytmgo-X.Y.Z-darwin-arm64.tar.gz` 与 `update-darwin-arm64.json`。签名覆盖压缩包的 SHA-256；包保留完整 `.app`、执行权限和内部符号链接。发布前使用公钥独立验签，并核对 GitHub 返回的每个资产摘要。失败会保留未公开 draft，不发布部分更新。修正失败后如已存在 draft/tag，先检查该 draft，使用新版本号重新构建，不覆盖已公开版本。

发布新稳定版：修改 `mygo.json` 和 `main.go` 的开发版本并推送 `main`，运行 **macOS package → notarized → publish_release**。或使用：

```sh
gh workflow run macos-package.yml --repo mreasonyang/ytmgo --ref main -f mode=notarized -F publish_release=true
```

仅测试打包时保持 `publish_release=false`，不会影响线上更新。验证预发行版时可填写 `release_version=0.5.0-rc.1`。客户端默认只接收稳定版，预发行版也能升级到版本号更高的稳定版。

0.4.0 需要从 [Releases](https://github.com/mreasonyang/ytmgo/releases) 手动安装一次。将应用从 DMG 拖入自己可写的 Applications 目录再运行；只读 DMG 内运行无法替换自身。安装 0.5.0 后启动自动检查，菜单可随时检查。更新窗口内可开启自动下载，默认先征询安装；更新替换 App bundle，不主动清理账号、语言或外观数据。

维护命令不启动音乐窗口或读取账号，用于隔离的已发布包验收：

```sh
/path/to/ytmgo.app/Contents/MacOS/ytmgo --version
/path/to/ytmgo.app/Contents/MacOS/ytmgo --check-for-updates
/path/to/ytmgo.app/Contents/MacOS/ytmgo --install-update
```

最后一个命令只在当前执行的 bundle 中暂存更新，进程退出后由 MyGo helper 替换；之后再次打开。正式验收应保留公开发布的旧版作为起点，验证公开 feed、下载签名、替换后的版本、Apple 签名和 stapled ticket；本地伪造旧版或源码测试不能代替这个结果。
