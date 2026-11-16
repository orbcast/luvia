# 项目上下文

## 定位

Luvia 是一个视频播放器项目。仓库按独立应用组织，允许 Go / MyGo、Swift / SwiftUI 等不同技术栈，技术选择在各应用内确定。当前可运行代码仍是 MyGo 桌面待办示例，视频播放尚未实现。根目录 `AGENTS.md` 规定共同治理原则，`apps/desktop/AGENTS.md` 保留该应用的 MyGo 原生 UI 与无窗口测试约束。

## 当前状态

- `apps/desktop` 是唯一可运行的应用：目前是待办清单示例（增、删、勾选完成、清除已完成、空状态），不是已实现的视频播放器。界面文案已接入多语言 catalogs（`en` 源语言、`zh-Hans`），品牌名 `Luvia` 不翻译。
- `apps/luvia-rules` 暂存视频播放器的源规则与资源源，目前仅有 README，尚无规则格式或加载实现，未来随时可能迁移至独立仓库。
- `apps/README.md` 导航应用与规则目录，`packages/` 仅有复用模块说明，尚无共享代码模块。Swift 等技术栈是未来选项，未创建工程。
- `docs/` 现有 [文档索引](docs/README.md)、[UI 术语表](docs/terminology.md) 与 [架构决策记录](docs/adr/README.md)；`DESIGN.md` 记录深浅主题配色与 Token，桌面示例尚未接入。
- 验证入口统一为 `make check`（MyGo vet、Go vet、测试、gofmt、仓库规则、catalogs）；`.githooks/pre-commit` 调用同一入口，`.githooks/commit-msg` 校验提交标题，CI 分三个作业（Linux 仓库规则、Linux 交叉 vet、macOS 应用检查/覆盖率/竞态/生成物/打包）；`scripts/check-rules.py` 承担 16 项可判定检查，`scripts/check-coverage.py` 统计手写代码覆盖率。
- 测试：`apps/desktop/main_test.go` 用 `ui.NewTester` 覆盖添加、空白输入、勾选与清理、删除后的顺序，以及界面文案跟随语言。
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

以下属于品牌图标的具体取舍，ADR 0004 只划边界，细节留在这里：图标统一用黑色版（`#080808` + 蓝色标志），不维护深浅两套，开发版靠应用名 “Luvia Dev” 区分；macOS 只给满版方图 + 矢量标志（`AppIcon.icon/` 的 `fill` 为黑、单图层 `mark.svg`），圆角遮罩、玻璃高光、投影交给系统图标栈，`mygo build` 用 `actool` 编译成 `Assets.car` 与 `AppIcon.icns`；超椭圆底图只用于没有系统遮罩的 Windows、Linux 与开发版（`resources/icon.png`）；母版集中在 `assets/icons/`（`mark.svg`、无损 WebP），`make icons` 同步，生成物提交但不手改，WebP 比同名 PNG 小约 40% 且解码后逐像素一致。应用身份 `com.example.luvia` 是模板占位符，`identifier` 同时决定数据目录与单实例锁。

`docs/` 与临时工作分离、技能放在 `.agents/skills/`（MyGo 两项由 `make skills` 更新，`use-iconify-skill` 自行维护）这两条沿用既有约定，未单独立 ADR。

## 已知限制

- 只在 macOS arm64 上验证过构建与测试；Windows/Linux 的打包、签名未验证。
- CI 定义已入库，但仓库没有 remote，尚未在远端真正运行过（包括交叉编译、覆盖率门槛与真实打包）；本地证书与公证同样未验证。pre-commit 与 commit-msg 需要每台机器执行一次 `make hooks`。
- 规则仍有一部分只能靠人判断（职责归属、设计语义、文案是否清晰），`scripts/check-rules.py` 只覆盖可判定的部分，不能当作全部合规的证明。
- `make icons` 依赖 `dwebp`（`brew install webp`）或带 Pillow 的 `python3`；生成物已入库，日常构建不需要这两个工具。
- 示例只覆盖基础控件；`ui.List`、`ui.Table`、多窗口、`mygo.WatchFiles` 等尚未出现在示例里。

## 下一步

- 按实际视频播放需求替换待办示例，遵循应用内的入口、UI、业务与适配职责划分，不将功能持续堆进 `main.go`。
- 按真实源规则需求定义 `luvia-rules` 的格式、接入和验证方式，保留随时迁移独立仓库的边界。
- 配置远端后让 CI 真正跑一次，并把结果写回本文件；发布时补 macOS 签名/公证与 Windows/Linux 打包验证。
- 出现需要两个以上应用共用的真实契约时，再按 ADR 0001 提取 `packages/` 模块。
