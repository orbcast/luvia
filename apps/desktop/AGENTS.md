# Desktop Agent 编码指南

本文件仅适用于 `apps/desktop/`，与根目录 [AGENTS.md](../../AGENTS.md) 一起读取。这里的 Go/MyGo 规则不适用于未来的 Swift 或其他应用。当前应用已实现导航、菜单与空业务区的桌面框架；本文件负责桌面应用的技术栈、代码位置、生命周期和验证命令。

## 技术栈与原生 UI 约束

- 使用 Go 1.27+、`github.com/egoist/mygo` 与 `ui` 包，实际版本以本目录 `go.mod` 为准。CLI 通过 `go tool` 固定，不依赖全局最新版。
- 界面只用 Go 与 `github.com/egoist/mygo/ui` 编写，由 MyGo 绘制；禁止 WebView、`webview` 标签、`mygo.config.ts`、Vite、Bun、npm、TypeScript、HTML、CSS、JS，不添加 `package.json` 或前端构建脚本。
- 不使用 `mygo.Bind` / `mygo.NewEvent` 生成 TypeScript 客户端驱动界面。应用逻辑直接调用 Go；富文本、表格、图表使用原生控件或自绘。
- 确需改变本应用的 UI 技术路线或引入 WebView/前端构建链时，先取得用户明确同意。新增独立 Swift 应用不属于改变本应用技术栈。
- 修改前读取相关代码、`mygo.json`、已有测试和文档。升级 MyGo 前通过 `go list -m -f '{{.Dir}}' github.com/egoist/mygo` 定位源码，读取相关 API 与 `docs/ui/migration.md`，不凭记忆改 API。
- 应用身份 `mygo.json` 的 `name`、`identifier`、`version`、`out` 保持稳定；改 `identifier` 会改变数据目录与单实例身份，须说明影响并在提交信息中记录。

## 代码归属与依赖

- `main.go` 只负责入口、平台 API 与依赖组装。`internal/appshell/` 放导航与菜单，`internal/appwindow/` 放主窗口生命周期，`internal/preferences/` 放主题偏好读写，`internal/design/` 放设计 Token；不把实现持续堆进入口。
- 复杂功能按需放入 `internal/<feature>/`，不预建目录、空接口或通用框架；搜索分区在 `internal/search/`，只经 `source.Source.List` 取数，不导入 `internal/catalog`。业务规则、用例与 I/O 适配按实际需要拆分，业务核心不依赖 `ui.Context`、`ui.Element` 或窗口。
- 源规则解析层固定为 `internal/source`（契约/错误分类）、`internal/source/rule`（解码校验）、`internal/source/engine`（解释器/HTTP）、`internal/source/loader`（扫描诊断），依赖单向；上游名称、密钥、端点只放 `apps/luvia-rules` 规则数据，不写进 desktop 源码。格式见 [docs/rule-dsl.md](../../docs/rule-dsl.md)。
- 行为测试集中在 `tests/`，通过生产包的公开接口验证；不在生产目录混放测试。夹具按需放 `tests/testdata/`，不把测试专用辅助逻辑塞进生产代码。
- `mygo.json`、`resources/`、`AppIcon.icon/` 放本端配置与资源，不放业务逻辑。共享素材遵循 [assets/README.md](../../assets/README.md)，已确认复用的能力遵循 [packages/README.md](../../packages/README.md)。

## 多语言与组件复用

- 用户可见文案、导航提示、无障碍名称和自定义菜单标签必须接入 MyGo 内置多语言功能，不在界面或动作代码中写死中文或英文；`scripts/check-rules.py` 会检查非测试 Go 代码里的中日韩字符串字面量。当前语言为英文（`en`，源语言）和简体中文（`zh-Hans`）。
- 文案在 `locales/en.json` 与 `locales/zh-Hans.json`，`mygo.json` 的 `i18n` 段指向该目录，`go tool mygo generate` 生成 `locales/catalog_gen.go`。界面只调用生成函数（如 `locales.Discover()`、`locales.Settings()`），改文案先改 catalogs 再生成，不自行实现平行字典或语言持久化。
- 品牌名 `Luvia` 不翻译，保留字面量；其余文案一律进 catalogs。语言选择与持久化复用 MyGo 提供的 API，切换后同步自定义菜单等一次性设置的标签。
- 测试用 `locales.In("en")` 取查找标签与断言文本，不写死中文；验证“界面跟随语言”时用 `i18n.SetLocaleFunc` 指向 `zh-Hans` 并在 `t.Cleanup` 恢复。流程与取舍见 [ADR 0005](../../docs/adr/0005-localized-messages.md)。
- 优先复用当前 MyGo 版本提供的控件、布局、输入、焦点和无障碍能力，减少自绘制组件。液态玻璃效果优先用官方 `plugins/glass` 材质或适用的窗口材质，不自绘一套玻璃效果。
- UI 命名复用 [UI 术语表](../../docs/terminology.md)；新增术语按根目录规则与用户确认并简短记录。

## 设计 Token 与界面图标

- 配色、尺寸与图像叠加 Token 遵循根目录 [DESIGN.md](../../DESIGN.md)，集中在 `internal/design/design.go`，AppShell 与各页面共用。组件引用集中 Token，不在 `view` 里散写色值。
- 用户调整设计时，同步根目录设计规范和本应用已实现的硬编码 Token、派生值与引用，并检查深浅主题。尚未接入的部分如实说明，不创建无消费者的常量文件。
- 界面操作图标用根目录规则指定的 Solar SVG，通过 [use-iconify-skill](../../.agents/skills/use-iconify-skill/SKILL.md) 脚本获取校验，母版放 `assets/ui-icons/solar/`；`go generate ./...` 从母版更新 `internal/icons/icons_gen.go`，图标名在 `internal/icons/icons.go` 解析缓存。
- SVG 通过固定 MyGo 版本的 `ui.ParseSVG` / `ui.MustParseSVG`、`ui.Icon` 等原生 API 接入，先核对源码用法；`currentColor` 由语义前景色驱动。资源应本地封装，不在每帧解析或联网下载，也不使用 WebView；品牌应用图标走 `make icons`。

## MyGo UI 生命周期与线程

详细指南见 [mygo-maintenance](../../.agents/skills/mygo-maintenance/SKILL.md)。通用 UI 设计原则遵循根目录规范。

- `view` 根据 app 状态构建控件，可能因一次输入重建多帧；不在无条件渲染代码中执行 I/O、启动 goroutine 或触发外部副作用，副作用由处理的动作或显式任务触发。
- 元素与构建期值会过期：不存进 app 状态、worker 或闭包跨帧使用，也不把 `ui.Context` 带入后台任务。持久身份使用 `.Key(key)`、`.Bind(&handle)`、`.FocusBind`。
- `Changed` / `Submitted` 在下一构建阶段报告，绑定值在构造后更新。派生值写回放在动作回调中，避免每帧计算覆盖用户输入。
- 状态变更在 UI 线程进行，慢任务放后台，通过 `Window.Update` 发布结果，取消或丢弃过期任务结果。不能用重绘请求代替状态同步。
- 遍历集合时延后增删，避免下标错位；长列表、表格使用 `ui.List`、`ui.Table`、`ui.Grid` 等虚拟化控件。

## MyGo 测试怎么做

- MyGo UI 测试优先用固定版本自带套件。业务用普通 Go `testing`；控件绑定、事件与状态用 `ui.NewTester`；运行中原生应用的菜单、快捷键、语言、主题和截图用官方 `plugins/automation` 与 `mygo-automation`，不先用 computer-use 等外部 GUI 框架。
- 使用 automation 前，通过 `go list -m -f '{{.Dir}}' github.com/egoist/mygo` 定位当前版本，读取其 `plugins/automation/cmd/mygo-automation/SKILL.md` 和 `docs/plugins/automation.md`，再核对 CLI `version` / `skills`。CLI 必须与应用依赖版本一致，从应用 module 构建或安装，不使用碰巧已安装的旧版。
- 原生用例见 [tests/native_test.go](tests/native_test.go)，入口 `make test-native`，启动见 [README.md](README.md)。`automation_e2e.go` 仅在 `e2e` 构建链接插件，用 `LUVIA_E2E_DATA_DIR` 隔离主题、语言和窗口状态；socket、日志、截图和构建放 OS 临时目录，结束清理。优先读语义树、稳定 key 和 `current=page`，操作后 `--settle`，等待条件而非 sleep。
- 只有套件未覆盖的系统窗口、Dock、系统弹窗等行为才使用外部 GUI 工具；先核实能力缺口。渲染需要时先用 automation 截图；执行菜单回调不等于验证菜单展开的外观。无窗口交互参考 [tests/app_shell_test.go](tests/app_shell_test.go)。
- 每个用例新建 app 与 `ui.NewTester(s.View, width, height)`，通过 `Click`、`Type`、`Key` 执行最短操作路径；检查操作返回的错误，同时断言 app 状态与 `HasText()` / `Texts()` 等用户可见结果。
- 直接更改预置状态后用 `Frame()` 构建界面；按固定版本的输入/通知语义推进必要帧，不用 sleep 或大量空帧等待。
- 仅对受影响行为选择导航选中、菜单同步、语言与偏好恢复等用例。焦点、快捷键、控件消失/重现与额外帧只在相关风险下验证。
- 不对无关文案、像素坐标、完整控件树或全屏截图建立脆弱断言；纯布局调整不要求新增截图测试。Tester 不证明 GPU 渲染、系统弹窗或平台集成正常，必要时另做针对性原生验证。
- Go 默认覆盖率不直接证明分支覆盖率；未测量的指标如实说明。风险分级、覆盖率要求、测试复用和停止条件遵循根目录规范。

## 验证命令

- Go 代码修改时先跑定向测试；每个可交付步骤完成后，在本目录执行 `CGO_ENABLED=0 go test ./...` 与 `go tool mygo vet .`。纯文档修改免跑应用检查；小幅外观修改保留已有无窗口测试和静态检查。
- 业务、接口或结构性代码改动增加 `go vet ./...`；共享状态、并发或后台任务改动增加 `go test -race ./...`，race 检查不能设置 `CGO_ENABLED=0`。
- 根目录 `make test` 默认检查本应用，`make check` 汇总 vet、test、格式、仓库规则、catalogs 与图标校验；覆盖率 `make cover`（生成代码不计，门槛 80%），竞态 `make race`，原生 UI 按条件用 `make test-native`。`.githooks/pre-commit` 与 CI 同入口。同一代码状态已通过的检查不重复跑；这些命令不检查其他应用或模块。
- 仅在明确渲染/平台风险、大范围布局/主题变更或用户要求时，用根目录 `make dev` 启动应用，验证受影响场景并结束本次启动的进程。不默认执行多尺寸截图和完整 GUI 回归。
- 打包、资源、发布或原生集成相关改动按需执行根目录 `make build`；产物在 `apps/desktop/build/<平台>/`，无法验证的平台与检查必须说明。

## 素材与诊断

- 图标母版统一在根目录 `assets/icons/`，通过根目录 `make icons` 生成本端 PNG 和同步 SVG，不手改生成物。
- 当前 macOS 图标使用满版底图与矢量标志，由系统提供遮罩和高光；Windows/Linux/开发版按既有方案使用带圆角的资源。具体流程见 [图标说明](../../assets/icons/README.md)。
- 用标准库 `log` / `log/slog` 写 stderr，不在 UI 塞调试文本，不遗留 `println`。日志用稳定 snake_case 字段和固定事件名，不记录敏感数据。
- 开发版由根目录 `make dev` 启动，日志打印到终端；打包版默认使用 info 级别，排查时临时调到 debug。
