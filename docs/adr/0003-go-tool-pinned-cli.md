# 0003 MyGo CLI 由 go tool 固定

- 状态：已采纳
- 日期：2026-11-16

## 背景

MyGo 仍在快速演进，命令行参数、生成物和平台构建流程会随版本变化。若依赖全局安装的 `mygo`，两台机器可能跑不同版本，生成出来的代码与提交内容不一致，问题还很难复现。与此同时，MyGo 要求 Go 1.27+，而本机可能装着更旧的工具链。

## 决定

`apps/desktop/go.mod` 用工具依赖固定 CLI：`tool github.com/egoist/mygo/cmd/mygo`，所有调用写成 `go tool mygo ...`（`Makefile` 里 `MYGO ?= go tool mygo`）。不使用全局安装，也不把 `mygo` 加进 `PATH` 约定。

Go 版本由 `go` 指令与 `GOTOOLCHAIN=auto` 决定：本机工具链较旧时由 Go 自己下载匹配版本，不需要手工升级。

## 后果

克隆仓库后无需额外安装步骤，`make dev`、`make build`、`go tool mygo i18n check` 在任何机器上版本一致；代价是第一次构建要下载 MyGo 及其依赖的工具链，离线环境需要预先缓存。升级 MyGo 变成一次显式的 `go get` 加重新生成提交，评审时能看到版本变化与生成物差异，而不是"某台机器上的 mygo 变了"。
