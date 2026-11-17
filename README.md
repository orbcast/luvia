# Luvia

Luvia 是一个视频播放器项目，按独立应用和共享能力组织代码，允许各应用选择适合自身需求的语言与框架，例如 Go / MyGo 或 Swift / SwiftUI。共同治理原则见根目录 [AGENTS.md](AGENTS.md)，技术专属约束由各应用维护；正式架构、功能、契约与指南文档统一由 [docs/README.md](docs/README.md) 导航，供 Agent 与人类共同参考。

当前可运行代码是 Go + [MyGo](https://mygo.run) 桌面框架，已实现左侧导航、原生窗口顶栏、系统菜单、主题和语言选择，以及发现、资源库、详情与播放页面，视频播放走来源规则解析与系统视频组件。该应用采用 MyGo 自绘的原生 UI，不加载网页、不使用 WebView；目前没有 Swift 工程。应用导航与新增应用约定见 [apps/README.md](apps/README.md)，桌面使用说明见 [apps/desktop/README.md](apps/desktop/README.md)。以下环境、命令和 MyGo 开发说明均针对当前桌面应用。

## 环境要求

- Go 1.27 或更高版本（`go version` 检查；低版本会由 `GOTOOLCHAIN=auto` 自动切换）。
- MyGo CLI 已在 `apps/desktop/go.mod` 中固定为 go tool，构建和开发不需要全局安装。
- 首次运行 `make doctor` 检查本机条件。

## 目录结构

```
apps/                  独立应用，可使用不同语言和工程形式
  README.md            应用导航、新应用约定与依赖边界
  desktop/              MyGo 桌面应用（Go module，独立构建与发布）
    README.md           本端环境、命令与平台说明
    AGENTS.md           Go/MyGo 专属编码与测试约束
    main.go             入口、平台 API 与依赖组装
    internal/appshell/  应用Tabbar、菜单与设计 Token
    internal/appwindow/ 主窗口关闭与重新激活
    internal/preferences/ 主题偏好读写
    tests/              独立的行为测试，通过公开接口验证实现
    locales/            多语言 catalogs（en 源语言 + zh-Hans）与生成的 Go 文案函数
    mygo.json           应用名、identifier、版本、图标、产物目录、catalogs 目录
    AppIcon.icon/       macOS 图标文档（macos.icon）：系统遮罩 + 矢量标志
    resources/icon.png      通用图标（Windows/Linux/开发版，黑色超椭圆）
    build/              打包产物（已忽略）
  luvia-rules/          视频播放器的源规则与资源源，未来随时可能独立拆仓
    README.md           用途、维护边界与迁移预期
packages/              确有多个消费者的可复用模块，按语言选择工程形式
  README.md            模块归属、依赖边界与验证说明
assets/                跨端共享的正式品牌与界面素材母版，禁止临时工作产物
  README.md            素材归属、格式与生成说明
  icons/               品牌图标源文件（矢量 SVG + 无损 WebP）+ 说明
  ui-icons/solar/      导航 Solar Linear SVG 母版与来源清单
scripts/icons.sh        图标母版 → 当前桌面应用图标文件（make icons）
scripts/check-rules.py  仓库规则检查：临时产物、文档链接与索引、AGENTS 预算、文案与配色字面量
.githooks/pre-commit    提交前跑 make check（克隆后执行一次 make hooks 启用）
.github/workflows/ci.yml CI：文档规则在 Linux、应用检查在 macOS 上执行
docs/                  Agent 与人类共同参考的正式专题文档
  README.md            文档归属、索引、查阅与维护约定
  adr/                 架构决策记录：为什么这样定、代价与重新评估条件
.agents/skills/         智能体技能（MyGo 维护与原生模块、通用 Iconify 脚本）
AGENTS.md               跨应用治理、解耦、设计同步、临时目录与风险分级测试
DESIGN.md               深浅主题语义 Token、配色与跨应用设计同步规范
CONTEXT.md              项目背景、已定决策与历史验证限制
Makefile                当前 MyGo 桌面应用的开发、测试、打包入口
```

[luvia-rules](apps/luvia-rules/README.md) 暂放在 `apps/` 下，用于维护播放器的源规则和资源源，目前仅有说明，未来随时可能迁移至独立仓库；它不是可运行的播放器应用。

以后增加其他应用时，在 `apps/` 下新建独立工程，维护本地 README 与 `AGENTS.md`，记录技术栈、状态/线程模型和验证命令，不互相引用内部实现，也不要求所有端都是 Go module。

多个实际消费者共用的能力放入 `packages/<name>/`，可采用 Go module、Swift Package 等合适形式，明确语言、平台与依赖边界；目前仅含说明。仅供一个应用使用的实现留在该应用中，具体规则见 [可复用模块说明](packages/README.md)。跨语言复用不等于直接互相导入，需要实际需求和明确边界。共享品牌素材统一放在 `assets/`，见 [共享素材说明](assets/README.md)。

## 设计与界面图标

界面配色与深浅主题的语义 Token 见 [DESIGN.md](DESIGN.md)。设计变化须同步该文档与受影响应用的实际 Token 定义；当前桌面框架已接入画布、导航与主题 Token。用户提供的参考图片不放入项目，Agent 任务文件和一次性脚手架使用 OS 临时目录，详见 [AGENTS.md](AGENTS.md) 与 [素材目录约束](assets/README.md)。

操作、导航与状态图标统一使用 Solar SVG，通过可复用的 [use-iconify-skill](.agents/skills/use-iconify-skill/SKILL.md) 查询、分页、下载和离线校验。技能支持其他 Iconify 集合，本项目的 Solar 限制由根目录 `AGENTS.md` 规定；品牌应用图标继续使用下文的独立流程。导航 SVG 已存放在 `assets/ui-icons/solar/`，由 `scripts/generate-desktop-icons.py` 生成桌面嵌入资源。

## 常用命令

以下命令默认服务 `apps/desktop`，不覆盖未来应用或全部共享模块；不要将其结果当作全仓验证。测试和 GUI 检查按改动风险选择，不要求每次全部运行。

```bash
make dev      # 启动开发版，改 Go 代码后自动重建
make test     # 跑当前桌面应用的无窗口测试
make test-native # MyGo 原生 UI 回归（先按 apps/desktop/README.md 启动隔离应用）
make build    # 打包当前平台到 apps/desktop/build/
make check    # 提交前检查：vet + test + 格式 + 仓库规则 + catalogs
make rules    # 只跑仓库规则检查（文档、命令、依赖方向、密钥、字面量）
make i18n     # 校验 catalogs 的占位符与复数形式（译文完整性由 make rules 拦）
make cover    # 语句覆盖率门槛，生成代码不计（CI 执行）
make race     # 竞态检测（CI 执行）
make check-gen # 确认 catalogs 与 Solar 生成代码同步（CI 执行）
make hooks    # 启用 pre-commit 与 commit-msg（每个克隆一次）
make icons-check  # 校验 assets/ui-icons 的 Solar 图标（尚无图标时跳过）
make icons    # 改了图标母版后同步到应用
make doctor   # 检查环境
make skills   # 更新 .agents/skills
```

等价的直接命令在 `apps/desktop` 下执行：`go tool mygo dev`、`CGO_ENABLED=0 go test ./...`、`go tool mygo build`、`go tool mygo vet .`。

## 桌面框架

主窗口提供发现、资源库、追剧、搜索、设置五个导航入口，点击、菜单和 `⌘1`～`⌘5` 同步选中状态，`⌘,` 进入设置；发现、资源库、详情与播放为已实现页面，追剧、搜索仍为占位。显示菜单可切换主题和语言并记住选择，系统菜单和窗口行为复用 MyGo。框架代码按 `internal/` 下的导航、窗口与偏好职责划分，`main.go` 只做组装，详见 [桌面说明](apps/desktop/README.md) 与 [ADR 0007](docs/adr/0007-desktop-shell.md)。

界面文案来自 `apps/desktop/locales/`（英文源语言加简体中文），品牌名 `Luvia` 除外。新增或修改文案时改 catalogs 后跑 `make i18n` 与 `go tool mygo generate`，流程见 [ADR 0005](docs/adr/0005-localized-messages.md)。`apps/desktop/tests/` 在独立目录验证导航、菜单、语言、布局、偏好和窗口重开，测试不启动原生窗口。

## 检查与提交

`make check` 是提交前唯一需要记住的入口：MyGo vet、Go vet、测试、gofmt、仓库规则（`scripts/check-rules.py` 的 16 项可判定检查）与 catalogs 校验。仓库规则不只查文档，还查文档代码块里的命令是否真实存在、apps 之间是否互相导入、有没有引入前端工程文件或 WebView 绑定、有没有私钥与本机路径、入口 `main.go` 是否超过 300 行。`.githooks/pre-commit` 调用 `make check`，`.githooks/commit-msg` 校验 `type(scope): 简述` 与 scope 白名单，首次克隆后执行一次 `make hooks` 同时启用两者。CI 分三个作业：Linux 跑仓库规则与 Windows/Linux 交叉 vet，macOS 跑 `make check`、覆盖率门槛（`make cover`，生成代码不计，门槛 80%）、竞态检测（`make race`）、生成物同步（`make check-gen`）与真实打包（`make build`）。检查失败时修代码或修脚本，不用 `--no-verify` 绕过。设计取向与检查边界见 [ADR 0006](docs/adr/0006-verification-gates.md)，决策清单见 [docs/adr/](docs/adr/README.md)。

仓库目前没有配置 remote，CI 定义尚未在远端真正运行过；本地能验证的是 `make check` 与钩子本身。

## 写自己的界面

1. 把可变状态放在明确的实例上（如 `appshell.Shell`），`View` 每帧只读状态、构建控件、处理本帧输入。
2. 需要持久身份（焦点、命令、滚动位置）时用 `.Key(key)`、`.Bind(&handle)`；元素本身会过期，不要存进状态或后台 goroutine。
3. 慢任务放后台 goroutine，结果通过 `mygo.Window.Update` 发布，并丢弃过期结果。
4. 长列表或表格用 `ui.List`、`ui.Table`、`ui.Grid`，不要手写大量 `Row`。

更多控件与约定见 [MyGo UI 文档](https://mygo.run/zh/docs/ui) 和 `.agents/skills/mygo-maintenance/SKILL.md`。

## 应用配置

`apps/desktop/mygo.json`：

```json
{
  "name": "Luvia",
  "identifier": "com.example.luvia",
  "version": "0.1.0",
  "out": "build",
  "i18n": {
    "dir": "locales",
    "source": "en"
  },
  "icon": "resources/icon.png",
  "macos": {
    "icon": "AppIcon.icon"
  }
}
```

- `identifier` 决定应用数据目录与单实例身份，正式使用前改成自己的反向域名；改名会让旧数据目录失效。
- `i18n.dir` 指向 catalogs 目录，`source` 是源语言；改这两项会改变生成代码与语言回退行为。
- 原生界面应用不需要 `mygo.config.ts`，也不需要 Node 工具链。

### 图标

品牌图标的源文件统一放在 `assets/icons/`，供以后所有端复用：

| 文件 | 说明 |
|---|---|
| `mark.svg` | 矢量标志（透明底）：蓝色圆球 + 镂空播放键，分辨率无关，是图标的唯一图形源 |
| `mark.webp` | 同一标志的 1024×1024 栅格版（无损 WebP，比 PNG 小约 40%） |
| `app-icon-dark.webp` | 黑色超椭圆（圆角方）底图 + 标志，默认应用图标，Windows/Linux/开发版用 |
| `app-icon-light.webp` | 白色超椭圆底图，备选配色 |
| `app-icon-dark-fullbleed.webp` | 黑色满版方图，备选配色，给需要系统遮罩的平台 |

标志在画布中约占 67%，四周留约 16% 内边距。改图标只改母版，再同步到应用：

```bash
make icons    # mark.svg -> AppIcon.icon/Assets/mark.svg；app-icon-dark.webp -> resources/icon.png
```

同步出来的文件要提交进仓库（这样没有 `dwebp` 的机器也能直接构建），但不要手改。转码用 `dwebp`（`brew install webp`），没有时回退到带 Pillow 的 `python3`。

应用里图标按平台分工，`mygo build` 会自动转换，不需要手工生成 `.ico` / 各种尺寸：

| 用途 | 文件 | 说明 |
|---|---|---|
| macOS | `AppIcon.icon/` | Icon Composer 文档：黑底 + 矢量标志，交给系统做遮罩与高光 |
| Windows、Linux、开发版 | `resources/icon.png` | 1024×1024 黑色超椭圆底图，`mygo build` 生成 Windows 执行文件图标、Linux 桌面图标 |

macOS 图标**只提供方图（满版）+ 矢量标志**，圆角遮罩、玻璃高光、投影全部由系统图标栈渲染——不要自己在图形里画圆角超椭圆或高光，否则会和系统遮罩叠成双层边。`mygo.json` 的 `macos.icon` 指向 `AppIcon.icon/`，`mygo build` 用 Xcode 的 `actool` 把它编译成 `Assets.car`（macOS 26 及以上）和 `AppIcon.icns`（更早系统的回退），需要 Xcode 26 或更高版本；没有 Xcode 的机器会跳过图标编译并回退到 PNG 渲染的 `.icns`。因为标志是 SVG，正式包的图标在任何尺寸下都不会糊。

图标不使用深浅色两套：`make dev` 的开发版走 MyGo 的另一条路径，只把 PNG 渲染成 `.icns`，拿不到 Icon Composer 文档，两套外观会变成开发版和正式包对不上。开发版用同一个 `resources/icon.png`（黑色超椭圆，这些平台没有系统遮罩，所以圆角烘在图上），靠应用名 “Luvia Dev” 区分；正式包才用系统遮罩。换图标时改 `assets/icons/` 的母版 → `make icons` → `make build`；用 Xcode 里的 Icon Composer 可以直接预览效果。

## 打包与发布

```bash
cd apps/desktop
go tool mygo build                    # 当前平台
go tool mygo build -platform darwin/arm64 -skip-dmg
```

产物在 `apps/desktop/build/<平台>/`。macOS 分发需要签名与公证（`mygo.json` 的 `macos.signingIdentity`），Windows 与 Linux 的打包细节见 [MyGo CLI 文档](https://mygo.run/zh/docs/cli)。

## 约束

当前桌面应用只能用 Go 与 `github.com/egoist/mygo/ui` 编写界面，禁止 WebView、Web 前端及通过 `mygo.Bind` 生成 TypeScript 客户端驱动界面，详见 [桌面应用约束](apps/desktop/AGENTS.md)。其他应用遵循自身技术规范；共同目录、解耦、UI 简洁原则与风险分级验证要求见根目录 [AGENTS.md](AGENTS.md)。
