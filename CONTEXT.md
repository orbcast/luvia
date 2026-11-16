# 项目上下文

## 定位

Luvia 是一个视频播放器项目。仓库按独立应用组织，允许 Go / MyGo、Swift / SwiftUI 等不同技术栈，技术选择在各应用内确定。当前可运行代码仍是 MyGo 桌面待办示例，视频播放尚未实现。根目录 `AGENTS.md` 规定共同治理原则，`apps/desktop/AGENTS.md` 保留该应用的 MyGo 原生 UI 与无窗口测试约束。

## 当前状态

- `apps/desktop` 是唯一可运行的应用：目前是待办清单示例（增、删、勾选完成、清除已完成、空状态），不是已实现的视频播放器。
- `apps/luvia-rules` 暂存视频播放器的源规则与资源源，目前仅有 README，尚无规则格式或加载实现，未来随时可能迁移至独立仓库。
- `apps/README.md` 导航应用与规则目录，`packages/` 仅有复用模块说明，尚无共享代码模块。Swift 等技术栈是未来选项，未创建工程。
- `docs/README.md` 是人类与 Agent 共同查阅的正式专题文档入口，目前只有目录说明；`DESIGN.md` 记录深浅主题配色与 Token，桌面示例尚未接入。
- `.agents/skills/use-iconify-skill/` 包含可复用的 Iconify 搜索、显式分页、SVG 下载、离线校验脚本和测试。本项目界面图标统一 Solar，尚未添加实际界面 SVG；品牌图标流程独立。
- 测试：`apps/desktop/main_test.go` 用 `ui.NewTester` 覆盖添加、空白输入、勾选与清理、删除后的顺序。
- 历史验证记录（不代表本轮重新执行）：`CGO_ENABLED=0 go test ./...`、`go tool mygo vet .`、`go tool mygo build -skip-dmg`（darwin/arm64，产出 `Luvia.app` 约 12 MB，含 `Assets.car`）。
- 仓库已完成首次 `git init`，后续变更按根目录 `AGENTS.md` 的原子提交约定维护。

## 技术决策

- **分层技术治理**：根规范保持跨技术栈；Go + MyGo 原生 UI、禁止 WebView 和前端构建链的要求只约束 `apps/desktop`，写在该应用的 `AGENTS.md`。未来独立应用可采用 Swift 等技术，并维护自己的框架、线程、测试和构建规范。
- **独立应用工程**：应用放在 `apps/<name>`，各自构建与发布，不共享内部实现，不强制都是 Go module。确有至少两个实际消费者后才提取 `packages/<name>`，按语言维护依赖与兼容性。
- **规则源可拆仓**：`apps/luvia-rules` 管理源规则和资源源，保持自包含，不依赖桌面内部实现；未来随时可能独立维护，接入通过明确格式或契约完成。
- **CLI 用 go tool 固定**：`apps/desktop/go.mod` 里 `tool github.com/egoist/mygo/cmd/mygo`，因此 `go tool mygo ...` 在任何机器上版本一致，不依赖全局安装。
- **技能放在仓库根 `.agents/skills/`**：MyGo 两项技能由 `mygo install-skills .` 安装（`mygo-maintenance`、`mygo-native-modules`），后续用 `make skills` 更新；自行维护的通用 `use-iconify-skill` 独立保留，脚本和测试放在该技能内部，不写本项目专属策略。
- **正式文档与临时工作分离**：`docs/` 供人类与 Agent 共同参考，设计 Token 以根 `DESIGN.md` 为准；设计变化同步实际应用 Token。参考图不存项目；任务服务文件、一次性脚手架、下载样本与测试报告等放 OS 临时区域，不以隐藏目录或忽略规则规避。
- **界面图标与品牌图标分离**：界面操作采用 Solar Linear/Bold 单色 SVG，必须通过 Iconify 技能脚本获取和校验，正式使用时存 `assets/ui-icons/solar/`，保留来源与许可，`currentColor` 绑定语义前景色；品牌应用图标继续使用 `assets/icons/`。
- **Go 1.27+（当前 MyGo 应用）**：MyGo 要求的最低版本；`GOTOOLCHAIN=auto` 会在本机工具链较旧时自动下载。
- **图标统一用黑色版**：黑底（`#080808`）+ 蓝色标志；不维护深浅色两套（`make dev` 的开发版只把 PNG 渲染成 `.icns`，拿不到 Icon Composer 文档，两套外观会和正式包对不上），开发版靠应用名 “Luvia Dev” 区分。
- **macOS 只给满版方图 + 矢量标志**：`AppIcon.icon/` 的 `fill` 是黑色，单个图层是 `mark.svg`（矢量，画布 1200 px 让标志约占 67%），圆角遮罩、玻璃高光、投影全部交给系统图标栈——不自己画超椭圆圆角或高光，否则会和系统遮罩叠成双层边。`mygo build` 用 `actool` 编译成 `Assets.car`（macOS 26+）和 `AppIcon.icns`（旧系统回退）；actool 接受 SVG 图层，图标在任何尺寸下都清晰。
- **超椭圆底图只用于没有系统遮罩的平台**：Windows、Linux 与开发版用 `resources/icon.png`（黑色超椭圆，圆角烘在图上，边缘与高光保留原图的，不重画）。
- **图标源文件集中到仓库根 `assets/icons/`**：图形源是矢量 `mark.svg`（512 视图 / 1200 px 画布），栅格母版是无损 WebP（`mark.webp`、`app-icon-dark.webp`，另备白底与满版两版），供后续所有端复用；构建链只吃 PNG，所以用 `make icons`（`scripts/icons.sh`）把 `mark.svg` 同步进 Icon Composer 文档、把 `app-icon-dark.webp` 转成 `resources/icon.png` 并提交，生成物不手改。WebP 比同名 PNG 小约 40%，解码后逐像素一致。
- **应用身份 `com.example.luvia`**：模板占位符，正式项目需要替换；`identifier` 同时决定数据目录与单实例锁。

## 已知限制

- 只在 macOS arm64 上验证过构建与测试；Windows/Linux 的打包、签名未验证。
- 未接入 CI，也没有公证/自动更新配置。
- `make icons` 依赖 `dwebp`（`brew install webp`）或带 Pillow 的 `python3`；生成物已入库，日常构建不需要这两个工具。
- 示例只覆盖基础控件；`ui.List`、`ui.Table`、多窗口、`mygo.WatchFiles` 等尚未出现在示例里。

## 下一步

- 按实际视频播放需求替换待办示例，遵循应用内的入口、UI、业务与适配职责划分，不将功能持续堆进 `main.go`。
- 按真实源规则需求定义 `luvia-rules` 的格式、接入和验证方式，保留随时迁移独立仓库的边界。
- 需要发布时补 macOS 签名/公证与 Windows/Linux 打包验证。
- 在已初始化的 Git 仓库中继续按功能切片验证和提交；远端配置与推送按用户后续要求执行。
