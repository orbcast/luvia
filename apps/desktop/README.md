# Desktop

Luvia 的 Go / MyGo 桌面应用。目前实现应用框架、发现页（Home）、资源库页（Library）、详情页（Detail）与播放页（Player）。

## 界面与交互

主窗口在 macOS 保留系统原生红黄绿按钮（`TitleBarHidden`，不隐藏系统按钮），由 AppKit 负责悬停提示、双击缩放、绿灯菜单和全屏显隐，应用不画窗口按钮；Windows 与 Linux 隐藏系统按钮并自己画：标题栏右端（桌面把按钮排在左端时改贴左端）一颗悬浮胶囊，内含最小化、最大化、关闭三枚 38 × 32 按钮，图标为 16 × 16 的 Solar Linear 横线与叉加一枚自绘方框（未最大化时单框，已最大化时斜叠双框，即还原态），悬停叠白底、关闭键悬停转红；胶囊材质见 `internal/glass`。播放页占用窗口期间这三枚自绘按钮让位给画面（`Shell.SetWindowControlsHidden`，由路由的 `Deps.WindowButtons` 在进入/离开播放页时切换），离开播放页立即恢复；macOS 的原生按钮始终留在左上角，进入全屏时由系统自行隐藏。窗口顶线 `ChromeTop` 在系统标题栏按钮下方留 12 个单位的空隙，与面板距窗口边缘的 12 一致，左列导航与详情页返回按钮都不会贴到红黄绿上；最小 400 × 400。[AppShell / 应用Tabbar](../../docs/terminology.md) 是顶部对齐的圆角玻璃导航，依次提供发现、资源库、追剧、搜索、设置；窗口宽度小于 720 逻辑单位时同一块玻璃改为底部 Dock，两种布局之间用 MyGo 转场（260ms 弹簧）连续移动，按钮与图标跨布局保持同一 `Key`。每次启动默认发现，点击、系统菜单和 `⌘1`～`⌘5` 同步切换选中项；`⌘,` 进入设置。切换页面使用 MyGo 转场（淡入 220ms、上移 10 个单位），不自绘动画；选中导航项为透明灰色玻璃底、图标换成 Solar Bold 实心版本并取当前主题前景色。

发现页由三块组成：Hero 轮播（取首页分区里横幅最多的分区，最多 4 项，6 秒自动切换、悬停暂停、右侧缩略图手动切换）、继续观看与本周追剧。Hero 背景是当前 banner 封面的高斯模糊铺底，文案区左侧是一张 2:3 封面容器（高度取窗口高度 38%，顶边与 AppShell 同处窗口顶线 `ChromeTop`，文案底边坐在封面底边上，banner 高度由「顶线 + 封面 + 48 呼吸空间」推出），右侧依次为标题、徽章、简介与操作；简介固定三行、超出以省略号截断，轮播缩略图与文案底边同线，模糊底用线性蒙版淡出并叠一层终点等于页面底色的渐变，与内容区无缝衔接。继续观看与本周追剧只读本地库，没有记录时显示空态，不填演示数据；周历按本地日期汇总追剧记录。

系统菜单包含 Luvia、编辑、显示、窗口；标准行为交由 MyGo role 处理，显示菜单提供导航、外观、语言及全屏。外观支持跟随系统、浅色、深色；语言支持跟随系统、简体中文、English。语言与窗口位置由 MyGo 持久化，主题保存在 `mygo.App.Path(mygo.PathUserData)` 下的 `theme.json`。首次启动与偏好文件缺失、损坏时打开深色（`internal/preferences/preferences.go` 的 `defaultTheme`），选择仍由设置页与菜单写回；写入失败只影响下次启动，本次切换仍生效并记录日志。

窗口关闭后可通过 Dock 再次激活，或通过菜单导航重建窗口。应用身份仍为 `com.example.luvia`，未改变数据目录。

## 代码与资源

`main.go` 只组装窗口、生命周期和平台 API。`internal/appshell/` 管理导航与系统菜单，`internal/appwindow/` 管理窗口关闭和重新激活，`internal/preferences/` 处理主题偏好读写，`internal/design/` 集中配色、尺寸与图像叠加 Token，`internal/glass/` 提供窗口控件胶囊的半透明表面（填充、描边、高光、投影加背景模糊）；行为测试集中于 `tests/`，通过公开接口调用生产实现。配色与尺寸遵循 [DESIGN.md](../../DESIGN.md)。

发现页与本地数据分成三块：`internal/home/` 是页面状态机与视图（拉取首页、轮播、周历聚合、空态与提示），只依赖 `source.Source`、本地库与图片加载器；`internal/catalog/` 是资源库页的状态机与视图（分类页签、搜索、筛选、海报网格/列表），只读源列表与本地追剧集合，加载用请求代号丢弃过期回包；`internal/library/` 保存观看进度与追剧，以 `source.Ref` 为键原子写入用户数据目录的 `library.json`；`internal/media/` 负责下载远程图片、在主线程解码，并用 `sha256(url)` 命名做内存与磁盘缓存（单张上限 12 MB、并发 4、磁盘默认 128 MiB、解码位图默认 96 MiB，两份都按最近使用时间淘汰）。`internal/player/` 是播放页状态机与视图（解析线路、切集、弹幕与进度），`internal/videocache/` 在回环上起 HTTP 代理做磁盘缓存，播放地址经它改写后再交给系统视频组件，一集解析成功后还会在后台预热下一集的前几个分片。使用 MyGo `ButtonBase`、`Icon`、原生窗口顶栏及官方 `plugins/glass` 材质；窗口按钮在 Windows 与 Linux 由应用自己画（系统按钮已隐藏），其胶囊用 `internal/glass` 的填充、描边、高光、投影叠官方 `Blur`，不自绘玻璃路径；macOS 用系统原生按钮，故不绘制胶囊。

中英文文案位于 `locales/en.json`、`locales/zh-Hans.json`，执行 `go tool mygo generate` 更新 `locales/catalog_gen.go`。自定义菜单在语言变化后重建，标准 role 标签由框架翻译。

Solar 图标母版及来源清单位于 [assets/ui-icons/solar](../../assets/ui-icons/solar/README.md)。在本目录执行 `go generate ./...` 从母版生成 `internal/icons/icons_gen.go`，`internal/icons` 负责按名称解析并缓存；只修改母版，不手改生成物。图标随应用嵌入，不依赖运行时网络，`currentColor` 跟随语义前景色。

## 应用图标与启播动画

应用图标母版在仓库根目录 `assets/icons/`，`make icons` 生成 `resources/icon.png`（Windows/Linux/开发版）并同步 macOS 的 `AppIcon.icon/Assets/mark.svg`。启播动画由 `internal/playintro` 负责：在进入播放页开始播放时，在窗口上覆盖一层深色背景并用 `ui.Video` 播放内嵌的 `playintro.apng`（球体从小放大后淡入 Luvia 字样），放完再回到界面；它不随应用启动播放。动画内嵌、不依赖运行时文件或网络；解码失败时直接跳过，不阻塞播放动作。

动画母版与生成脚本见 [assets/brand/playintro/README.md](../../assets/brand/playintro/README.md)，改图标或动画后运行 `make brand` 重新生成，不手改生成物。`internal/playintro` 的自检在 `go test ./...` 中解码内嵌动画，防止空资源；无窗口环境可用 `xvfb-run` 起虚拟屏后台验证启播动画，不干扰真实桌面。

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

`make build` 打包当前平台并把 `apps/luvia-rules` 内置为应用包资源，产物在 `build/`；打包后的应用不依赖环境变量即可读到源规则，用户数据目录的 `rules/` 或 `LUVIA_RULES_DIR` 仍可覆盖。规则里以 `${env:NAME}` 求值的密钥由「设置 → 访问密钥」提供，存进用户数据目录的 `secrets.json`，启动时补进进程环境（已有的同名变量优先），因此从开始菜单或 Finder 双击启动也能用，见 [ADR 0019](../../docs/adr/0019-access-keys-in-the-app.md)。无窗口测试不证明 GPU 材质、系统菜单或其他平台渲染正确，涉及这些边界的变化需做对应原生验证。技术规则见 [AGENTS.md](AGENTS.md)，设计取舍见 [ADR 0007](../../docs/adr/0007-desktop-shell.md)。
