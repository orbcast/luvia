SHELL := /bin/bash

APP ?= apps/desktop
MYGO ?= go tool mygo

.DEFAULT_GOAL := help
.PHONY: help doctor dev test build vet fmt check skills icons

## help: 列出可用命令
help:
	@echo "Luvia · 可用命令"
	@echo
	@grep -hE '^## [a-z-]+:' $(MAKEFILE_LIST) | sed -E 's/^## ([a-z-]+): */  make \1 — /' | sort

## doctor: 检查本机是否满足 MyGo 的开发条件
doctor:
	@cd $(APP) && $(MYGO) doctor

## dev: 启动开发版应用，改 Go 代码后自动重建
dev:
	@cd $(APP) && $(MYGO) dev

## test: 运行全部测试（无需窗口）
test:
	@cd $(APP) && CGO_ENABLED=0 go test ./...

## build: 打包当前平台的生产应用到 apps/desktop/build/
build:
	@cd $(APP) && $(MYGO) build

## icons: 从 assets/icons 的共享母版重新生成各应用的 PNG 图标
icons:
	@sh scripts/icons.sh

## vet: 运行 MyGo 的静态检查与 go vet
vet:
	@cd $(APP) && $(MYGO) vet . && go vet ./...

## fmt: 格式化 Go 代码
fmt:
	@gofmt -w $(APP)

## check: 提交前检查（格式、静态检查、测试）
check: vet test
	@unformatted="$$(gofmt -l $(APP))"; \
	if [ -n "$$unformatted" ]; then echo "需要 gofmt: $$unformatted"; exit 1; fi

## skills: 更新 .agents/skills 下随 CLI 分发的智能体技能
skills:
	$(MYGO) install-skills .
