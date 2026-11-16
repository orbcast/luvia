# Desktop

Luvia 的 Go / MyGo 桌面应用。目前实现应用框架，尚无视频播放或媒体业务内容。

## 界面与交互

主窗口保留系统红黄绿按钮与顶部拖动区域。[AppShell / 应用Tabbar](../../docs/terminology.md) 是左侧居中的圆角玻璃导航，依次提供发现、资源库、追剧、搜索、设置。每次启动默认发现，点击、系统菜单和 `⌘1`～`⌘5` 同步切换选中项；`⌘,` 进入设置。五个入口的业务内容当前均留空。

系统菜单包含 Luvia、编辑、显示、窗口；标准行为交由 MyGo role 处理，显示菜单提供导航、外观、语言及全屏。外观支持跟随系统、浅色、深色；语言支持跟随系统、简体中文、English。语言与窗口位置由 MyGo 持久化，主题保存在 `mygo.App.Path(mygo.PathUserData)` 下的 `theme.json`。偏好读取失败回退跟随系统，写入失败只影响下次启动，本次切换仍生效并记录日志。

窗口关闭后可通过 Dock 再次激活，或通过菜单导航重建窗口。应用身份仍为 `com.example.luvia`，未改变数据目录。

## 代码与资源

`main.go` 只组装窗口、生命周期和平台 API。`internal/appshell/` 管理导航、系统菜单与主题，`internal/appwindow/` 管理窗口关闭和重新激活，`internal/preferences/` 处理主题偏好读写；行为测试集中于 `tests/`，通过公开接口调用生产实现。配色集中在 `internal/appshell/theme.go`，尺寸集中在 `internal/appshell/design_tokens.go`，遵循 [DESIGN.md](../../DESIGN.md)。使用 MyGo `ButtonBase`、`Icon`、原生窗口顶栏及官方 `plugins/glass` 材质，不自绘玻璃或窗口按钮。

中英文文案位于 `locales/en.json`、`locales/zh-Hans.json`，执行 `go tool mygo generate` 更新 `locales/catalog_gen.go`。自定义菜单在语言变化后重建，标准 role 标签由框架翻译。

Solar Linear 图标母版及来源清单位于 [assets/ui-icons/solar](../../assets/ui-icons/solar/README.md)。在本目录执行 `go generate ./...` 从母版生成 `internal/appshell/icons_gen.go`；只修改母版，不手改生成物。图标随应用嵌入，不依赖运行时网络，`currentColor` 跟随语义前景色。

## 原生 UI 测试

优先使用 MyGo 自带套件：无窗口状态与交互用 `ui.NewTester`，运行中应用的菜单、快捷键、语言、主题和截图用 `mygo-automation`。版本必须匹配本应用的 `go.mod`；在应用 module 内构建工具，不依赖全局已安装版本。先读取该版本的 `plugins/automation/cmd/mygo-automation/SKILL.md` 与 `docs/plugins/automation.md`，CLI 的 `skills` 也提供同版指南。

以下在本目录的同一终端执行。测试构建的 `automation_e2e.go` 通过临时目录隔离应用数据，`MYGO_LOCALE=en` 固定起始语言且阻止语言写回；普通构建不链接插件。二进制、socket、日志及状态都在 OS 临时目录：

```sh
run_dir=$(mktemp -d)
go build -tags e2e -o "$run_dir/Luvia" .
go build -o "$run_dir/mygo-automation" github.com/egoist/mygo/plugins/automation/cmd/mygo-automation
export LUVIA_E2E_CLI="$run_dir/mygo-automation"
export LUVIA_E2E_SOCKET="$run_dir/app.sock"
export LUVIA_E2E_DATA_DIR="$run_dir/data"
MYGO_AUTOMATION="$LUVIA_E2E_SOCKET" MYGO_LOCALE=en "$run_dir/Luvia" >"$run_dir/app.log" 2>&1 &
app_pid=$!
trap 'kill "$app_pid" 2>/dev/null || true; rm -rf "$run_dir"' EXIT
```

每轮从上述新启动的英文测试应用开始，再执行 `make -C ../.. test-native`，以验证进程启动时默认发现。它运行独立目录中的 `tests/native_test.go`，验证默认发现、五个导航按钮、菜单和快捷键切换、设置入口、中英文菜单重建，以及三种主题实际写入偏好；通过 `current=page` 断言唯一选中项和空业务内容。CLI 会话自动关闭，退出测试终端时上述 trap 清理应用和目录。该测试需要已启动的图形会话，不加入无窗口的 `make check` 或默认 CI。

交互式排查可先执行 `"$LUVIA_E2E_CLI" open --socket "$LUVIA_E2E_SOCKET" --session luvia-debug`，再使用同一 session 的 `snapshot -i`、`press key=2 --settle`、`menu View Appearance Dark --settle`；只有渲染无法从语义树判断时使用 `screenshot` 写入临时目录。结束后执行同一 session 的 `close`。窗口关闭、Dock 与系统弹窗等套件不覆盖的行为才使用外部 GUI 工具；菜单命令验证回调，不能证明菜单展开的外观。

## 开发与验证

从仓库根目录运行 `make dev` 启动，`make check` 执行静态检查、无窗口行为测试、图标生成器及覆盖率脚本测试及仓库规则，`make cover` 检查手写 Go 覆盖率，`make race` 检查竞态，`make icons-check` 校验 Solar 来源与 SVG。`make check-gen` 在干净检出上确认 catalogs 和图标生成物同步；提交前执行 `make hooks` 安装钩子。

`make build` 打包当前平台，产物在 `build/`。无窗口测试不证明 GPU 材质、系统菜单或其他平台渲染正确，涉及这些边界的变化需做对应原生验证。技术规则见 [AGENTS.md](AGENTS.md)，设计取舍见 [ADR 0007](../../docs/adr/0007-desktop-shell.md)。
