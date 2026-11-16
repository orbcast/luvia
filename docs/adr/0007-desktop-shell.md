# 0007 桌面框架与应用内职责分层

- 状态：已采纳
- 日期：2026-11-16

## 背景

桌面待办示例需要替换为参考图确定的播放器框架。本轮只实现左侧 AppShell / 应用Tabbar、原生窗口顶栏与系统菜单，业务内容留空。用户要求生产实现有明确层级，测试与生产目录分开，并复用 MyGo 控件、玻璃材质与多语言能力。

## 决定

`apps/desktop/main.go` 只负责平台 API 与依赖组装。导航状态、界面、菜单和设计 Token 放 `internal/appshell/`，主窗口关闭与再次激活放 `internal/appwindow/`，主题偏好读写放 `internal/preferences/`；`tests/` 通过公开接口验证行为。窗口管理只定义 Show、Invalidate、OnClosed 三个必要方法，以便隔离原生资源测试关闭、重开和导航刷新，不引入通用窗口框架。

AppShell 固定五个导航入口，每次进程启动默认发现，点击、菜单和快捷键同步。系统菜单采用 Luvia、编辑、显示、窗口四组，标准行为使用框架 role。语言持久化由 MyGo 负责，自定义菜单随语言重建；MyGo v0.9.1 的 Theme.SetSource 不持久化，因此主题单独原子写入应用数据目录的 `theme.json`，失败时本次会话仍能切换。窗口身份与数据目录保持稳定。

五个 Solar Linear SVG 母版和来源清单位于 `assets/ui-icons/solar/`。生成器逐字嵌入 `internal/appshell/icons_gen.go`，启动时解析一次，通过 `currentColor` 跟随主题，不在每帧读取文件或依赖网络。整条导航使用官方 glass 材质，系统窗口按钮不自绘。尺寸与主题约定见 [DESIGN.md](../../DESIGN.md)。

## 后果

生产目录与测试目录分开后，覆盖率使用 `-coverpkg=./...`，保留入口在统计范围，脚本按代码范围合并多个测试程序的重复记录；阈值仍为 80%，生成代码仍排除。生成器和覆盖率脚本均有独立回归测试，`make check-gen` 同时检查 catalogs 与 SVG 生成物。

公开接口仅覆盖当前导航、菜单、偏好和窗口生命周期；没有建立空业务层或共享包。后续播放、资源库与源规则按实际业务职责新增模块，不能继续堆进入口或应用Tabbar。

UI 验证优先使用固定版本自带套件：`ui.NewTester` 验证无窗口状态和回调，`mygo-automation` 驱动实际应用的菜单、导航、快捷键、语言与主题。`automation_e2e.go` 只在 `e2e` 构建接入插件并允许隔离应用数据；`tests/native_test.go` 通过同版本 CLI 驱动已启动的测试应用，根目录 `make test-native` 执行，默认无窗口检查与 CI 不启动原生应用。只有套件未覆盖的平台行为才使用外部 GUI 工具；语义树不能证明 GPU 材质或菜单展开外观，按风险补充截图或平台验证。
