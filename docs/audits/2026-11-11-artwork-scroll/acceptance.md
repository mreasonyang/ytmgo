# 封面滚动缓存验收

2026-11-11，macOS arm64，运行时提交 `27481e8`，MyGo `v0.7.2`。验收覆盖自动回归、隔离的原生 GPU 窗口和真实匿名首页，使用临时开发构建。

## 自动回归

| 检查 | 结果 |
| --- | --- |
| `go test -count=1 ./...` | 21 个包，1,544 项测试/子测试通过，40 项跳过，0 失败；34.253 秒 |
| `go test -race -count=1 -timeout=600s ./...` | 21 个包，1,543 项通过，40 项跳过，0 失败/竞态；410.189 秒 |
| `go test -race -run '^TestArtwork' -count=1 -timeout=120s .` | 79 项通过，1 个独立子进程 helper 入口跳过，0 失败/竞态；41.797 秒 |
| `go vet ./...`、`go tool mygo i18n check` | 通过 |
| `CGO_ENABLED=0 GOARCH=amd64 GOOS=linux go build ./...` | 通过，仅编译 |
| `CGO_ENABLED=0 GOARCH=amd64 GOOS=windows go build ./...` | 通过，仅编译 |

完整 race 二进制生成后新增了一项高熵压缩数据压力用例；它已在上述 79 项封面专项 race 及单独 race 中通过，最新普通全量也包含该用例。所有最终检查使用相同的冻结产品代码。跳过项主要为显式启用的联网、账号、播放和本机运行时集成用例；没有为本轮开启这些账号测试。

回归覆盖：60 张 384px 高清封面和 600 次精确位图复用；三个池互不挤占；相同 rendition 的播放器/浏览共享；同一不可改尺寸 URL 的 128/384/768 并发请求只下载一次正文；首次绘制后释放像素并保留压缩后备；压缩字节及临时像素压力；取消、重试、无效/超限图像；ETag/Last-Modified/304、Age、Expires、max-age 优先级、no-cache/no-store/private、Vary 变体及 200/304 更改 Vary；公开响应撤回缓存许可；损坏数据恢复；512 URL 分组/64 MiB 配额及 80% 低水位；低可用空间和回收后复查；磁盘不可写时继续显示；24 个独立实例及 8 个真实子进程并发写入。

`TestArtworkWarmNavigationFirstPresentedFramePaintsCover` 在下一轮构建界面时检查上一帧已完成的栅格图。这使 Tester 完成布局期间的中间帧也进入采样。精确采样数和高熵压力用例字节见 `regression-metrics.json`。

初次 race 找到 MyGo v0.7.1 软件栅格恢复像素的竞态：并行 tile 会同时触发延迟恢复。升级到上游 v0.7.2 后，相同滚动测试原样通过；上游在并行绘制前恢复像素，未跳过或禁用该测试。一次旧 180 秒全量 race 超时也未计为通过；上表为最终完整结果。

## 实际 macOS GPU 窗口

使用 `artwork_desktop_acceptance_test.go` 的 opt-in `TestMain`，运行真正的 MyGo 原生窗口和产品 `nativeView`/封面缓存。虚构目录含 60 张首页卡片、240 行歌曲和三个播放器封面；传输为确定性公开图片 fixture，不启动产品账号或播放控制器。显示器比例实测为 2×；窗口实际切换 1440×940、760×740、420×740 DIPs。布局 fixture 关闭动画，避免把动画时间混入缓存断言。

通过原生 CUA 滚动遍历首页与完整长列表；随后三轮执行首页/长列表滚动往返、20 次快速页面离开/返回、播放器切换、面板/沉浸模式和重复缩放。工具截图确认封面实际绘制；工具某次 30 秒调用超时后重连并继续，产品进程未退出。

| 热缓存指标 | 实测 |
| --- | --- |
| 绘制路径 | 489/489 帧为 GPU |
| 已缓存封面请求 | 5,869 次，其中 3,507 次为高清请求 |
| 已缓存请求的占位返回 / 位图身份改变 | 0 / 0 |
| HTTP 正文 | 冷预热 304 次；三轮后仍为 304，热阶段新增 0 次 |
| 帧耗时 P50 / P95 / P99 / 最大 | 0.8 / 10.5 / 18.8 / 78.8 毫秒 |
| 最终 thumbnail / browse / player 条目 | 241 / 60 / 3 |
| 三池最终压缩后备计费合计 | 443,008 字节；未释放像素计费均为 0 |

位图身份使用 `uintptr` 记录，不额外持有位图来制造内存稳定。占位计数是缓存调用层观测；中间栅格像素测试和实际窗口截图提供独立绘制证据，未逐帧录制屏幕，不能声称排除了所有显示层瞬时空白。

帧数字来自 MyGo 的 build/layout/paint/present 统计，不是输入到屏幕发光的端到端延迟，也不代表每帧均低于 16.7ms。冷预热期间仍有后台测试，不将其当作严格性能对照。

## 内存

使用仓库 `scripts/diagnostics/measure.py` 的 PID/可执行文件及数值 rusage 收集器，约 300 秒采样；只观测该 QA 进程，所有样本覆盖完整。没有读取进程参数、环境或原始诊断日志。

| 周期 | 操作期间 physical footprint 峰值 | 周期结束 |
| --- | --- | --- |
| 1 | 332.50 MiB | 129.22 MiB |
| 2 | 303.27 MiB | 119.08 MiB |
| 3 | 300.52 MiB | 111.75 MiB |

最终 Go heap allocation 为 14.35 MiB。操作峰值包含窗口/GPU/运行时资源，明显高于封面源数据池计费；本次短期重复操作没有显示逐轮累积，不能据此声称 112 MiB 是硬上限，或已完成长时间泄漏验收。汇总见 `desktop-metrics.json`；逐秒中间数据留在临时 QA 目录，不纳入仓库。

## 真实匿名首页及隔离边界

另以生产 `main()` 构建开发程序，仅用 Go overlay 追加启动初始化，将 MyGo 用户数据、缓存和日志路径固定为新建的 `/tmp` 目录。正式应用没有替换。最终 v0.7.2 构建实际加载匿名首页、下滚触发后续目录、返回首页；初始已加载歌曲和卡片封面仍正常显示。保持 Sign in / Not playing，独立目录的会话文件在测试前后均不存在。真实联网目录的冷图允许出现加载占位；此路径没有精确正文或逐帧计数，定量结论来自上面的原生 fixture。

一次早期启动仅通过环境变量启用隔离。退出后的 CUA 状态查询自动重开该旧 bundle，未继承变量，短暂触发了应用自身对原有 Dev 账号的恢复尝试。立即停止该确切进程，删除旧可执行文件并将 overlay 改为每次启动都固定隔离路径；后续重开及最终复验保持匿名。未点击账号控件，也未检查正常账号、认证存储或配置文件；因此不声称早期重开完全没有触发应用自身的认证存储访问。最终两个 QA 可执行文件均经 `safe-processes` 确认已退出。

## 复现与证据

在仓库根目录运行 `python3 docs/audits/2026-11-11-artwork-scroll/run_checks.py --check go-tests`；同一脚本支持 `go-race`、`artwork-race`、`go-vet`、`i18n`、`linux-build` 和 `windows-build`。脚本移除已知集成测试开关的名称后正常继承环境，只保留结构化状态/计数，不保存原始输出。

原生 fixture：`go test -c -o /tmp/ytmgo-artwork-qa.test .`，再设置一个全新的绝对 `YTMGO_ARTWORK_DESKTOP_ACCEPTANCE_DIR` 并以 `MYGO_FRAME_STATS=all` 运行该测试二进制。它不运行普通测试，而在主线程启动原生窗口。打包临时 `.app` 时，在 `Contents/Resources/artwork-acceptance-directory` 写入该独立目录，使自动重开仍保持隔离；正式构建不带此 marker。窗口菜单提供指标复位及三种验收尺寸，Explore/Back 切换虚构页面。

结构化检查结果为本目录的 `go-tests.json`、`go-race.json`、`artwork-race.json`、`go-vet.json`、`i18n.json`、`linux-build.json`、`windows-build.json`；源码绑定见 `source-hashes.json`，真实 main 隔离信息见 `live-main-isolation.json`。未验证 Windows/Linux 原生 UI、真实账号播放、长时间运行或安装包升级。
