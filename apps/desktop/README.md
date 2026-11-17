# Desktop

Luvia 的 Go / MyGo 桌面应用。目前实现应用框架与发现页（Home），视频播放与详情页尚未接入。

## 界面与交互

主窗口保留系统红黄绿按钮与顶部拖动区域，最小 400 × 400。[AppShell / 应用Tabbar](../../docs/terminology.md) 是顶部对齐的圆角玻璃导航，依次提供发现、资源库、追剧、搜索、设置；窗口宽度小于 720 逻辑单位时同一块玻璃改为底部 Dock，两种布局之间用 MyGo 转场（260ms 弹簧）连续移动，按钮与图标跨布局保持同一 `Key`。每次启动默认发现，点击、系统菜单和 `⌘1`～`⌘5` 同步切换选中项；`⌘,` 进入设置。切换页面使用 MyGo 转场（淡入 220ms、上移 10 个单位），不自绘动画；选中导航项为透明灰色玻璃底、图标换成 Solar Bold 实心版本并取当前主题前景色。

发现页由三块组成：沉浸式 Hero 轮播（取首页分区里横幅最多的分区，最多 4 项，6 秒自动切换、悬停暂停、右侧缩略图手动切换）、继续观看与本周追剧。Hero 图像用线性蒙版淡出并叠一层终点等于页面底色的渐变，白色标题带柔和投影，亮色海报也能读清。继续观看与本周追剧只读本地库，没有记录时显示空态，不填演示数据；周历按本地日期汇总追剧记录。主播放、影片详情与收藏按钮分别触发回调或写入本地库，播放器和详情页尚未实现时用短暂提示如实说明。资源库、追剧、搜索与设置仍为占位页。

系统菜单包含 Luvia、编辑、显示、窗口；标准行为交由 MyGo role 处理，显示菜单提供导航、外观、语言及全屏。外观支持跟随系统、浅色、深色；语言支持跟随系统、简体中文、English。语言与窗口位置由 MyGo 持久化，主题保存在 `mygo.App.Path(mygo.PathUserData)` 下的 `theme.json`。偏好读取失败回退跟随系统，写入失败只影响下次启动，本次切换仍生效并记录日志。

窗口关闭后可通过 Dock 再次激活，或通过菜单导航重建窗口。应用身份仍为 `com.example.luvia`，未改变数据目录。

## 代码与资源

`main.go` 只组装窗口、生命周期和平台 API。`internal/appshell/` 管理导航与系统菜单，`internal/appwindow/` 管理窗口关闭和重新激活，`internal/preferences/` 处理主题偏好读写，`internal/design/` 集中配色、尺寸与图像叠加 Token；行为测试集中于 `tests/`，通过公开接口调用生产实现。配色与尺寸遵循 [DESIGN.md](../../DESIGN.md)。

发现页与本地数据分成三块：`internal/home/` 是页面状态机与视图（拉取首页、轮播、周历聚合、空态与提示），只依赖 `source.Source`、本地库与图片加载器；`internal/library/` 保存观看进度与追剧，以 `source.Ref` 为键原子写入用户数据目录的 `library.json`；`internal/media/` 负责下载远程图片、在主线程解码，并用 `sha256(url)` 命名做内存与磁盘缓存（单张上限 12 MB、并发 4）。使用 MyGo `ButtonBase`、`Icon`、原生窗口顶栏及官方 `plugins/glass` 材质，不自绘玻璃或窗口按钮。

中英文文案位于 `locales/en.json`、`locales/zh-Hans.json`，执行 `go tool mygo generate` 更新 `locales/catalog_gen.go`。自定义菜单在语言变化后重建，标准 role 标签由框架翻译。

Solar 图标母版及来源清单位于 [assets/ui-icons/solar](../../assets/ui-icons/solar/README.md)。在本目录执行 `go generate ./...` 从母版生成 `internal/icons/icons_gen.go`，`internal/icons` 负责按名称解析并缓存；只修改母版，不手改生成物。图标随应用嵌入，不依赖运行时网络，`currentColor` 跟随语义前景色。

## 应用图标与开屏动画

应用图标母版在仓库根目录 `assets/icons/`，`make icons` 生成 `resources/icon.png`（Windows/Linux/开发版）并同步 macOS 的 `AppIcon.icon/Assets/mark.svg`。开屏动画由 `internal/splash` 负责：`main.go` 启动时先显示一个 512×512 的无边框深色窗口，用 `ui.Video` 播放内嵌的 `opening.apng`（球体从小放大后淡入 Luvia 字样），播放结束再打开主窗口并安装菜单。动画内嵌、不依赖运行时文件或网络；解码失败时直接进入主窗口，不会卡住启动。

动画母版与生成脚本见 [assets/brand/opening/README.md](../../assets/brand/opening/README.md)，改图标或动画后运行 `make brand` 重新生成，不手改生成物。`internal/splash` 的自检在 `go test ./...` 中解码内嵌动画，防止空资源；无窗口环境可用 `xvfb-run` 起虚拟屏后台验证开屏与主界面，不干扰真实桌面。

## 源规则解析层

播放器不直接实现任何站点。`apps/luvia-rules/<source-id>/rule.json` 用声明式 JSON 描述上游，本应应用通过 `internal/source` 的五个操作（首页、列表、详情、选集、解析）读取，规则格式与字段见 [docs/rule-dsl.md](../../docs/rule-dsl.md)。

解析层分四个包：`internal/source` 定义契约、`Ref` 与错误分类，不依赖 HTTP；`internal/source/rule` 严格解码并校验规则；`internal/source/engine` 执行请求、签名、翻页、字段映射和线路兑换；`internal/source/loader` 扫描目录、跳过坏规则并收集诊断，支持热重载。引擎不缓存、不聚合，也不自动续期直链。

规则目录按 `LUVIA_RULES_DIR`、用户数据目录的 `rules/`、应用包内 `Resources/luvia-rules/` 的顺序解析；开发态指向仓库里的 `apps/luvia-rules`，`mygo.json` 的 `resources` 把该仓库目录原样复制进应用包，因此 `make dev` 与 `make build` 的产物都自带规则。规则来自运行时文件，不编译进二进制，因此拆仓后无需改代码。

`make rules-verify` 用规则自带的 `fixtures/` 离线跑通每个源，不访问真实上游；这条通路同时用一份 `maccms` 形状的假上游证明 DSL 不是为单一站点定制的。

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

从仓库根目录运行 `make dev` 启动：它会把 `LUVIA_RULES_DIR` 指向仓库内的 `apps/luvia-rules` 并打印实际目录，因此开发版开箱即可读到 kanju 源；用 `LUVIA_RULES_DIR=... make dev` 可以换成别的规则目录。`make check` 执行静态检查、无窗口行为测试、图标生成器及覆盖率脚本测试及仓库规则，`make cover` 检查手写 Go 覆盖率，`make race` 检查竞态，`make icons-check` 校验 Solar 来源与 SVG。`make check-gen` 在干净检出上确认 catalogs 和图标生成物同步；提交前执行 `make hooks` 安装钩子。

`make build` 打包当前平台并把 `apps/luvia-rules` 内置为应用包资源，产物在 `build/`；打包后的应用不依赖环境变量即可读到源规则，用户数据目录的 `rules/` 或 `LUVIA_RULES_DIR` 仍可覆盖。无窗口测试不证明 GPU 材质、系统菜单或其他平台渲染正确，涉及这些边界的变化需做对应原生验证。技术规则见 [AGENTS.md](AGENTS.md)，设计取舍见 [ADR 0007](../../docs/adr/0007-desktop-shell.md)。
