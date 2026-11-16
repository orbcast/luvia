# MyGo 桌面示例

本应用使用 Go 1.27+ 与 MyGo 原生 UI，实现待办添加、删除、完成切换、清除已完成和空状态。依赖与 CLI 版本以 [go.mod](go.mod) 为准，不加载网页，不使用 WebView。

## 结构与约束

`main.go` 当前包含状态、动作、view 和窗口入口，`main_test.go` 使用 `ui.NewTester` 做无窗口交互测试。`mygo.json` 保存身份与构建配置，`resources/` 与 `AppIcon.icon/` 保存本端资源，`build/` 为已忽略的打包产物目录。功能增长时按本地 [AGENTS.md](AGENTS.md) 的职责规则拆分。

## 开发与验证

从仓库根目录执行：

```sh
make doctor
make dev
make test
make check
make build
```

这些入口默认服务本应用；`make check` 汇总 MyGo vet、Go vet、测试和格式检查。也可以在本目录执行固定版本 CLI：

```sh
CGO_ENABLED=0 go test ./...
go tool mygo vet .
go vet ./...
go tool mygo dev
go tool mygo build
```

以上命令按任务选择，不要求每次全部执行。共享状态或后台任务变化时增加 `go test -race ./...`，不能设置 `CGO_ENABLED=0`；纯文档修改只做文档检查，小幅外观调整不默认启动完整 GUI 回归。详细验证范围见本地与根目录规范。

## 平台与资源

产物位于 `build/<平台>/`。历史构建验证范围为 macOS arm64，不能据此认定其他平台或当前改动已经验证；发布前按目标平台验证工具链、签名与打包。macOS 图标编译有额外工具要求，见 [共享图标说明](../../assets/icons/README.md)。

图标母版从根目录 `assets/icons/` 获取，在仓库根目录用 `make icons` 同步，不手改生成物。根目录 [README.md](../../README.md) 提供当前图标和打包细节。
