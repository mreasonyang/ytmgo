# 官方 YouTube Music 桌面网页离线存储核验

核验日期：2026-11-04。证据级别：当前官方静态源码确认；未完成真实账号下载及断网验收。

结论：官方当前桌面播放器的新建离线下载路径包含媒体分块加密落盘、读取后解密，使用 AES-CTR 和 IndexedDB。证据涉及媒体字节，不仅是曲目元数据。

## 直接证据

以下行号来自本次直接获取的原始脚本，未重新排版。

| 环节 | 官方文件与位置 | 核验结果 |
| --- | --- | --- |
| Music 接入 | offline.js:303-310、364 | 音乐曲目下载动作进入共享媒体下载任务。 |
| 初始化 | base.js:7621-7624 | 新建媒体写入器准备随机密钥与初始计数值；续传可恢复已有参数。 |
| 媒体写入 | base.js:4129-4131、3634-3636 | 音视频分块经过加密，再写入数据库的 media 存储区。 |
| 加密实现 | base.js:2192、3985-3986、6795-6796 | WebCrypto 使用 AES-CTR；另有 JavaScript 实现。 |
| 本地数据库 | base.js:7126 | 数据库基名为 yt-player-local-media，包含媒体、索引和字幕存储区。 |
| 离线读取 | base.js:3638-3640、7534-7537 | 按区间读取本地分块，带加密参数时先解密，再交给播放器。 |
| 到期处理 | offline.js:233 | 过期策略会禁用离线播放。 |

这说明可以对标其受控离线机制，不需要把用户引导回官方 PWA。源码也保留无加密参数的读取分支，因此本次不宣称所有历史缓存均采用相同形式。上述证据不等于硬件 DRM、不可提取保证或第三方下载授权。

## 媒体密钥位置

同日继续核对静态调用链：播放器把媒体密钥及初始计数值作为本地媒体描述中的两个参数，连同格式信息写入上述数据库的 index 存储区；加密媒体分块位于 media 存储区。对应 base.js:4124-4125、3634-3635；通用数据库包装器在 6509-6512 直接写入 IndexedDB，没有额外编码媒体索引。

因此，这条实现中的媒体密钥随本地媒体索引持久化，并未在该路径调用系统 Keychain。其他浏览器或磁盘保护不在此次源码核验范围内。

生成来源也已确认：base.js:7624 在新下载时分别调用浏览器侧随机生成函数准备密钥与初始计数值；1698-1699 显示其优先使用浏览器密码学随机源。续传时从已有本地媒体描述恢复参数（4122、7624）。这两个参数用于本地缓存封装，不是服务器下发的通用内容解锁密钥。ytmGO 可独立生成自己的随机密钥，无需复用 PWA 已有密钥；服务端的媒体访问、下载资格及有效期控制是另一层机制。

ytmGO 可以采用不同设计：独立离线主密钥放 Keychain，曲目密钥经其保护后落盘，媒体采用分块认证加密。对“只复制应用缓存目录”的攻击，这可提供更强的密钥隔离；它不代表整个客户端已比浏览器更安全，也不能防止获授权的播放进程在运行时接触明文。目前仓库已有账号会话的 Keychain 接入，媒体密钥管理尚未实现；不应复用账号会话密钥。

参考：[Apple Keychain 服务](https://developer.apple.com/documentation/security/keychain-services)、[Keychain 访问控制](https://developer.apple.com/documentation/security/access-control-lists)。

## 来源与复核

- [官方桌面界面脚本](https://music.youtube.com/s/ad9b01d0/music_polymer_inlined_html.js)
- [官方播放器 base.js](https://music.youtube.com/s/player/11a45ffb/player_es6.vflset/zh_CN/base.js)
- [官方离线模块 offline.js](https://music.youtube.com/s/player/11a45ffb/player_es6.vflset/zh_CN/offline.js)

通过 Brave 的开发者工具确认官方页面实际加载的脚本地址，再直接获取公开静态文件。未读取 Cookies、认证存储、真实密钥或签名媒体地址。

临时源码快照：`/tmp/ytmgo-pwa-research.IDc8xi/`，可能随系统清理消失。

| 文件 | SHA-256 |
| --- | --- |
| music.js | 4654aee5ba5418a3dc8c5356f4cabadbc5b49dba0e9cc7c670412754ee908810 |
| player.js | 20d50057e867cd3d718fde8d985673b91a5ae29f20a179e48bebb257925ef462 |
| offline.js | 2ec28b640896d256969ec8a7fb2773cbc3db22aaa9d78c0ff2a48544c920d420 |
