# 项目上下文

## 定位

Luvia 是一个视频播放器项目。仓库按独立应用组织，允许 Go / MyGo、Swift / SwiftUI 等不同技术栈，技术选择在各应用内确定。当前可运行代码是 MyGo 桌面框架、发现页（Home）、资源库页（Library）、详情页（Detail）与播放页（Player），视频播放已接入来源解析与系统视频组件，重复观看经本地缓存代理。根目录 `AGENTS.md` 规定共同治理原则，`apps/desktop/AGENTS.md` 保留该应用的 MyGo 原生 UI 与无窗口测试约束。

## 当前状态

- `apps/desktop` 是唯一可运行的应用：已实现应用Tabbar（发现、资源库、追剧、搜索、设置）、原生顶栏、系统菜单、主题和语言选择，以及发现页与资源库页。发现页含 Hero 轮播（背景为当前封面的高斯模糊铺底，文案左侧为封面容器，6 秒自动、悬停暂停、缩略图切换）、继续观看、本周追剧与空态；资源库页按分类彩色页签、筛选行与海报网格浏览源列表，默认「全部」分类与网格视图；追剧、搜索仍为占位、设置页提供外观与语言，占位页标题按页面边距排布，不与 AppShell 重叠。搜索页已接入源规则：一个关键词输入框（回车提交、输入停顿 300 ms 自动搜索、本次会话最近 8 个关键词作为可直接再搜的 chip）、结果海报网格与追剧书签，空态、加载、失败重试与窄布局各自有状态。发现页的「本周追剧」在源规则声明 `updates` 块时按天向上游要当日更新单（每天 6 张带“更新至第 N 集”状态行，点击进详情），某天失败或上游无数据时退回本地追剧记录，二者皆空显示空态。窗口宽度小于 720 逻辑单位时 AppShell 从左侧列变为底部 Dock，同一块玻璃与同一组按钮经 MyGo 转场连续移动；窄布局下页面边距收窄、Hero 文案换行、封面缩小、卡片与海报列数随宽度下降。界面文案使用多语言 catalogs（`en` 源语言、`zh-Hans`），品牌名 `Luvia` 不翻译。
- 发现页与资源库页的数据分两层：片单来自源规则的首页与列表操作，观看进度与追剧来自本地库 `library.json`（以 `source.Ref` 为键），远程图片由 `internal/media` 下载并缓存；详情页读取来源的详情与选集并渲染海报、标签、简介、选集网格与右栏资料，播放页解析选中线路后交给系统视频组件播放，并把观看进度写回本地库；播放地址经 `internal/videocache` 的本地回环代理改写，HLS 子清单、分片与密钥落盘缓存，首帧到达前显示缓冲提示；线路解析出的地址先看前 64 字节的真实内容再决定是否播放，`Content-Type` 只在字节说不清时参考，因此贴图伪装成 mp4 或视频顶着 `.jpg` 后缀与 `image/jpeg` 都不会误判，前几条线路都不给媒体时退回默认线路；发现页与资源库的播放、详情入口都进入详情页。
- `apps/luvia-rules` 定义并维护源规则：一条规则是纯数据 JSON，由一个上游目录（`rule.json` + `fixtures/` + `README.md`）组成。目前已接入 `kanju/`（首页、列表与搜索、当日更新、详情、选集、线路解析），格式见 [源规则格式](docs/rule-dsl.md)，未来随时可能迁移至独立仓库；`updates` 块让首页周表显示上游当周的更新单，`list.searchIntent` 让同一个列表块按有无关键词在浏览与搜索之间切换。
- `apps/README.md` 导航应用与规则目录，`packages/` 仅有复用模块说明，尚无共享代码模块。Swift 等技术栈是未来选项，未创建工程。
- `docs/` 现有 [文档索引](docs/README.md)、[UI 术语表](docs/terminology.md)、[源规则格式](docs/rule-dsl.md) 与 [架构决策记录](docs/adr/README.md)；`DESIGN.md` 记录深浅主题配色与 Token，桌面框架已接入。
- 验证入口统一为 `make check`（MyGo vet、Go vet、测试、gofmt、仓库规则、catalogs）；`.githooks/pre-commit` 调用同一入口，`.githooks/commit-msg` 校验提交标题，CI 分三个作业（Linux 仓库规则、Linux 交叉 vet、macOS 应用检查/覆盖率/竞态/生成物/打包）；`scripts/check-rules.py` 承担 16 项可判定检查，`scripts/check-coverage.py` 统计手写代码覆盖率。
- 测试：`apps/desktop/tests/` 通过生产包公开接口验证导航、菜单同步、快捷键、语言、布局、主题偏好、窗口重开、发现页与资源库页交互，包含一条 headless 像素回归（Hero 底边与页面背景色差、亮色海报上标题区亮度）；源规则用规则自带 fixtures 与一份 `maccms` 形状的假上游验证 DSL 的通用性，`make rules-verify` 离线执行。生产实现按 `internal/appshell/`、`internal/appwindow/`、`internal/preferences/`、`internal/design/`、`internal/home/`、`internal/catalog/`、`internal/detail/`、`internal/player/`、`internal/screens/`、`internal/library/`、`internal/media/`、`internal/videocache/` 分层，源规则解析层按 `internal/source`（契约）、`internal/source/rule`（解码与校验）、`internal/source/engine`（解释器）、`internal/source/loader`（目录与诊断）分层。
- 历史验证记录（不代表本轮重新执行）：`CGO_ENABLED=0 go test ./...`、`go tool mygo vet .`、`go tool mygo build -skip-dmg`（darwin/arm64，产出 `Luvia.app` 约 12 MB，含 `Assets.car`）。
- 仓库已完成首次 `git init`，尚未配置 remote；后续变更按根目录 `AGENTS.md` 的原子提交约定维护。

## 技术决策

决策的完整背景、代价与重新评估条件记在 [docs/adr/](docs/adr/README.md)，这里只留索引和最容易被问到的取舍：

- [ADR 0001](docs/adr/0001-apps-and-packages.md)：应用放 `apps/<name>` 各自独立，确有至少两个消费者才提取 `packages/<name>`；`apps/luvia-rules` 保持自包含便于拆仓。
- [ADR 0002](docs/adr/0002-layered-agent-rules.md)：规则按目录分层，根 9000、其他 6000 字符预算；能脚本化的条款进 `scripts/check-rules.py`；规则与代码冲突时在同一次变更里改掉一方。
- [ADR 0003](docs/adr/0003-go-tool-pinned-cli.md)：`apps/desktop/go.mod` 用 `tool github.com/egoist/mygo/cmd/mygo` 固定 CLI，调用写 `go tool mygo ...`，不依赖全局安装；Go 版本靠 `GOTOOLCHAIN=auto` 自动切换。
- [ADR 0004](docs/adr/0004-interface-and-brand-icons.md)：界面操作图标（Solar SVG、`currentColor`、`assets/ui-icons/solar/`、Iconify 脚本离线校验）与品牌应用图标（`assets/icons/` 母版 → `make icons` → Icon Composer/`actool` 或 `resources/icon.png`）是两条不同链路，要求不同、归属不同，因此各自成文，不合并也不互相替代。
- [ADR 0005](docs/adr/0005-localized-messages.md)：文案进 `apps/desktop/locales/`，`go tool mygo generate` 生成 `catalog_gen.go`，界面只调用生成函数；测试用 `locales.In("en")` 取文案，语言跟随用 `i18n.SetLocaleFunc`。
- [ADR 0006](docs/adr/0006-verification-gates.md)：`make check`、pre-commit、CI 共用一套命令；被拦下的问题修代码或修脚本，不绕过。
- [ADR 0007](docs/adr/0007-desktop-shell.md)：桌面框架按导航、窗口、偏好分层，生产实现与 `tests/` 分目录；复用官方控件、玻璃、role 与语言持久化，单独保存主题；Solar 母版生成本地嵌入资源。
- [ADR 0008](docs/adr/0008-rule-dsl-and-engine.md)：源规则用声明式 DSL（分块步骤、受限选择器、闭集函数），desktop 只做通用解释；规则目录运行时加载，不编译期嵌入，坏规则隔离并留诊断。
- [ADR 0009](docs/adr/0009-home-page-and-local-library.md)：发现页由源规则首页 + 本地观看库组合，Hero 取自横幅最多的分区并 6 秒轮播；进度与追剧只存本地、空态不造假数据；图片下载后在主线程解码并缓存。

以下属于品牌图标的具体取舍，ADR 0004 只划边界，细节留在这里：图标统一用黑色版（`#080808` + 蓝色标志），不维护深浅两套，开发版靠应用名 “Luvia Dev” 区分；macOS 只给满版方图 + 矢量标志（`AppIcon.icon/` 的 `fill` 为黑、单图层 `mark.svg`），圆角遮罩、玻璃高光、投影交给系统图标栈，`mygo build` 用 `actool` 编译成 `Assets.car` 与 `AppIcon.icns`；超椭圆底图只用于没有系统遮罩的 Windows、Linux 与开发版（`resources/icon.png`）；母版集中在 `assets/icons/`（`mark.svg`、无损 WebP），`make icons` 同步，生成物提交但不手改，WebP 比同名 PNG 小约 40% 且解码后逐像素一致。应用身份 `com.example.luvia` 是模板占位符，`identifier` 同时决定数据目录与单实例锁。

`docs/` 与临时工作分离、技能放在 `.agents/skills/`（MyGo 两项由 `make skills` 更新，`use-iconify-skill` 自行维护）这两条沿用既有约定，未单独立 ADR。

## 已知限制

- 只在 macOS arm64 上验证过构建与测试；Windows/Linux 的打包、签名未验证。
- CI 定义已入库，但仓库没有 remote，尚未在远端真正运行过（包括交叉编译、覆盖率门槛与真实打包）；本地证书与公证同样未验证。pre-commit 与 commit-msg 需要每台机器执行一次 `make hooks`。
- 规则仍有一部分只能靠人判断（职责归属、设计语义、文案是否清晰），`scripts/check-rules.py` 只覆盖可判定的部分，不能当作全部合规的证明。
- `make icons` 依赖 `dwebp`（`brew install webp`）或带 Pillow 的 `python3`；生成物已入库，日常构建不需要这两个工具。
- 桌面框架只覆盖导航、发现页、资源库页与应用边界；`ui.List`、`ui.Table`、多窗口、`mygo.WatchFiles` 等尚未使用。
- 图片缓存按 `sha256(url)` 落盘，磁盘默认 128 MiB、解码后的位图默认 96 MiB，两份预算都按最近使用时间淘汰，页面再次取用被淘汰的图片时从磁盘重解码而不走网络（细节见 [ADR 0012](docs/adr/0012-artwork-cache-budget.md)）；视频缓存有字节上限与 LRU（默认磁盘 2 GiB、单对象 512 MiB），按范围读取的对象以 `.cache.part` 常驻并同样计入预算与淘汰，HLS 的段与密钥同样缓存；播放页解析成功后会在后台预热下一集的前几个分片，非 HLS 地址不预热；预热写入的对象在没人读之前记为未观看，占用不超过预算四分之一，淘汰时先删未观看的再按 LRU 删读过的，所以回退历史数据不会因为一次预热而被挤掉（细节见 [ADR 0010](docs/adr/0010-video-cache-proxy.md)）。
- 真实 HLS 播放仍依赖系统解码器：macOS 的 AVFoundation 原生支持，Linux 需要 `gstreamer1.0-plugins-bad` 提供 hlsdemux，当前 Linux 开发机未安装且无 root，故真实 HLS 在本机无法播放（与缓存无关）。本机旧版 GStreamer/无 root 环境下连本地 mp4 也无法出首帧，Linux 的真实播放验证要换到 macOS 或装有完整插件的机器上做。本地库格式为 version 1 的 JSON，尚无迁移代码。
- 详情页与播放页已实现：弹幕优先按剧名与集数从站外弹幕源（`internal/danmaku`，当前是 B 站）取，取不到时回落到规则的 `comments` 块，评论页签按具体原因说明空在哪一步；弹幕只有开关，没有不透明度/字号/行数等设置项；字幕只有 `ToggleSubtitle` 开关、没有数据来源；画中画未实现（`design` 里的 `PlayerPill` 系列 Token 与 `icons.Weighed` 也还没有调用方，播放页仍用 `plugins/glass`）；评分、收藏数、放送资料等参考图字段上游未提供，界面按真实字段（类型、地区、年份、话数、导演、演员、标签）展示；片单聚合（跨源）尚未实现；资源库页只取列表首页、无分页加载更多；发现页只用一个源，Hero 的“横幅最多分区”是布局启发式而非规则概念。

## 下一步

- 为播放页接入字幕的数据来源（弹幕已按标题接站外源，见 [ADR 0015](docs/adr/0015-danmaku-from-outside-the-rule.md)），并把播放页进度写回接上继续观看的续播位置；追剧、搜索沿用同一套源与本地库。
- 把弹幕设置（不透明度、字号、行数、显示区域）与画中画接到界面，用上已入库的 `design.PlayerPill` 系列 Token 与 `icons.Weighed`。
- 设置页暴露规则目录配置项，替代目前的环境变量与默认目录约定；再按需要做跨源聚合与图片缓存淘汰。
- 配置远端后让 CI 真正跑一次，并把结果写回本文件；发布时补 macOS 签名/公证与 Windows/Linux 打包验证。
- 出现需要两个以上应用共用的真实契约时，再按 ADR 0001 提取 `packages/` 模块。
