# 第三方客户端离线音频存储核对

日期：2026-11-04。仅结论于下列固定源码路径与项目文档，未运行这些客户端执行真实下载，不代表所有版本、插件和客户端。

| 项目 | 当日 Stars / 维护证据（UTC） | 本次核对结果 |
| --- | --- | --- |
| Pear Desktop | 33,978；2026-11-01 翻译提交 | 下载插件解析媒体，默认 MP3 预设，可选源格式，经转换及标签处理后直接写文件；此路径未增加本地音频加密。 |
| Metrolist | 13,999；2026-11-02 播放修复；README 声明维护模式 | Media3 下载管理器写入独立下载缓存；标准文件缓存读写，没有额外音频加密数据源或写入器。 |
| SimpMusic | 12,727；2026-11-03 功能分支合并 | Android 为 Media3 播放/下载两套缓存，核对路径没有额外音频加密层。桌面下载处理器指向应用下载目录，上层选择音频并使用 WebM 扩展名；另有导出普通媒体文件功能。桌面底层写入函数未完全核验。 |

无额外音频加密层不等于 PCM；内容仍可为 AAC、Opus 等压缩媒体。缓存碎片不一定能单独打开播放，碎片格式也不构成加密。上述音频路径未呈现 PWA 那样的本地媒体密钥管理；不能据此判断账号凭据的存储安全。

## 固定源码

- Pear：提交 `3337446b448454125ad261099a25f6b10fbb8cb6`，[下载插件](https://github.com/pear-devs/pear-desktop/blob/3337446b448454125ad261099a25f6b10fbb8cb6/src/plugins/downloader/main/index.ts)，预设与取流 404-444 行，写文件 478-479 行。
- Metrolist：提交 `e6fea3b36c68841bad21daa6252e1d69ecd3afbe`，[缓存构造](https://github.com/MetrolistGroup/Metrolist/blob/e6fea3b36c68841bad21daa6252e1d69ecd3afbe/app/src/main/kotlin/com/metrolist/music/di/AppModule.kt) 74-96 行，[下载管理](https://github.com/MetrolistGroup/Metrolist/blob/e6fea3b36c68841bad21daa6252e1d69ecd3afbe/app/src/main/kotlin/com/metrolist/music/playback/DownloadUtil.kt) 96-104、237-245 行。
- SimpMusic：顶层提交 `013db77728e788ae9866d6fa763dfa5aa7025c1e` 对应 core 子模块 `bb681959532959305ac040c4bb9e42eff30bdb93`；[Android 下载](https://github.com/maxrave-dev/core/blob/bb681959532959305ac040c4bb9e42eff30bdb93/media/media3/src/main/java/com/maxrave/media3/service/download/DownloadUtils.kt) 50-73、178-186 行，[缓存构造](https://github.com/maxrave-dev/core/blob/bb681959532959305ac040c4bb9e42eff30bdb93/media/media3/src/main/java/com/maxrave/media3/di/Media3ServiceModule.kt) 118-134、577-589 行，[桌面处理器](https://github.com/maxrave-dev/core/blob/bb681959532959305ac040c4bb9e42eff30bdb93/media/media-jvm/src/main/java/com/simpmusic/media_jvm/download/DownloadUtils.kt) 93-102、223 行，[上层媒体下载](https://github.com/maxrave-dev/core/blob/bb681959532959305ac040c4bb9e42eff30bdb93/service/kotlinYtmusicScraper/src/commonMain/kotlin/com/maxrave/kotlinytmusicscraper/YouTube.kt) 2288-2380 行。
- [Media3 默认缓存读写说明](https://developer.android.com/reference/androidx/media3/datasource/cache/CacheDataSource.Factory)：默认读取为文件数据源，默认写入为缓存数据写入器。
- [SimpMusic 下载和缓存文档](https://www.simpmusic.org/docs/dev/downloads-and-cache)、[导出文件说明](https://www.simpmusic.org/docs/guide/downloads-offline)。

## 核验边界

SimpMusic 一份底层源文件的返回被敏感输出保护拦截。未恢复、复制或记录被拦截的值，也未以其他方式重新获取该文件。该层实现不作为“桌面下载完全无加密”的直接证据；表格保留此未验证范围。

三项目均未归档。Stars 是本次 GitHub API 快照；维护判断依据提交内容及 README，非 updated_at。GPL 项目只作设计参考，直接复用其代码另需遵守许可证。
