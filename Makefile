SHELL := /bin/bash

APP ?= apps/desktop
MYGO ?= go tool mygo
COVER_MIN ?= 80

.DEFAULT_GOAL := help
.PHONY: help doctor dev test build vet fmt check rules i18n cover race check-gen hooks icons icons-check skills

## help: 列出可用命令
help:
	@echo "Luvia · 可用命令"
	@echo
	@grep -hE '^## [a-z0-9-]+:' $(MAKEFILE_LIST) | sed -E 's/^## ([a-z0-9-]+): */  make \1 — /' | sort

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

## check: 提交前检查（格式、静态检查、测试、仓库规则、catalogs）
check: vet test rules i18n
	@unformatted="$$(gofmt -l $(APP))"; \
	if [ -n "$$unformatted" ]; then echo "需要 gofmt: $$unformatted"; exit 1; fi

## rules: 检查仓库规则（临时产物、文档、命令、依赖方向、密钥、文案与配色字面量）
rules:
	@python3 -B scripts/check-rules.py

## i18n: 校验多语言 catalogs 的占位符与复数形式（译文完整性由 make rules 检查）
i18n:
	@cd $(APP) && $(MYGO) i18n check

## cover: 统计手写代码的语句覆盖率（生成代码不计），低于门槛就失败
cover:
	@dir="$$(mktemp -d)"; trap 'rm -rf "$$dir"' EXIT; \
	cd $(APP) && CGO_ENABLED=0 go test -coverprofile="$$dir/cover.out" ./... > /dev/null && \
	python3 -B $(CURDIR)/scripts/check-coverage.py --profile "$$dir/cover.out" --min $(COVER_MIN)

## race: 用竞态检测器跑测试（较慢，CI 执行，不能设 CGO_ENABLED=0）
race:
	@cd $(APP) && CGO_ENABLED=1 go test -race ./...

## check-gen: 确认生成代码与 catalogs 一致（会重新生成，只在干净检出上执行）
check-gen:
	@cd $(APP) && $(MYGO) generate > /dev/null
	@if ! git -C $(CURDIR) diff --quiet -- $(APP); then \
		echo "生成物与 catalogs 不同步：本地跑 go tool mygo generate 后提交"; \
		git -C $(CURDIR) --no-pager diff --stat -- $(APP); \
		exit 1; \
	fi
	@echo "生成物与 catalogs 一致"

## hooks: 把仓库的 .githooks 设为 Git 钩子目录（每个克隆执行一次）
hooks:
	@git config core.hooksPath .githooks
	@echo "pre-commit（make check）与 commit-msg（提交信息规范）已启用"

## icons-check: 校验 assets/ui-icons 下 Solar 图标的来源、风格与 currentColor（尚无图标时跳过）
icons-check:
	@if [ -d assets/ui-icons/solar ]; then \
		python3 -B .agents/skills/use-iconify-skill/scripts/iconify.py check \
			--dest assets/ui-icons/solar --prefix solar \
			--style linear --style bold --require-current-color; \
	else \
		echo "没有 assets/ui-icons/solar，跳过图标校验"; \
	fi

## skills: 更新 .agents/skills 下随 CLI 分发的智能体技能
skills:
	$(MYGO) install-skills .
