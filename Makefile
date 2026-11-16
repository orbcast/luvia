SHELL := /bin/bash

APP ?= apps/desktop
MYGO ?= go tool mygo
COVER_MIN ?= 80

.DEFAULT_GOAL := help
.PHONY: help doctor dev test test-native build vet fmt check rules rules-verify i18n cover race check-gen hooks icons icons-check skills

## help: 列出可用命令
help:
	@echo "Luvia · 可用命令"
	@echo
	@grep -hE '^## [a-z0-9-]+:' $(MAKEFILE_LIST) | sed -E 's/^## ([a-z0-9-]+): */  make \1 — /' | sort

## doctor: 检查本机是否满足 MyGo 的开发条件
doctor:
	@cd $(APP) && $(MYGO) doctor

## dev: 启动开发版应用，自动注入仓库内源规则，改 Go 代码后自动重建
dev:
	@rules="$${LUVIA_RULES_DIR:-$(CURDIR)/apps/luvia-rules}"; \
	if [ ! -f "$$rules/kanju/rule.json" ]; then \
		echo "dev: 警告：$$rules 下没有可用的规则目录，界面会显示“没有可用数据源”"; \
	else \
		echo "dev: 源规则目录 $$rules（覆盖方式：LUVIA_RULES_DIR=... make dev）"; \
	fi; \
	cd $(APP) && LUVIA_RULES_DIR="$$rules" $(MYGO) dev

## test: 运行全部测试（无需窗口）
test:
	@cd $(APP) && CGO_ENABLED=0 go test ./...
	@python3 -B -m unittest discover -s scripts -p 'test_*.py'

## test-native: 通过匹配版本的 mygo-automation 验证已启动的隔离原生测试应用
test-native:
	@cd $(APP) && CGO_ENABLED=0 go test -tags e2e ./tests -run '^TestNativeShell$$' -count=1 -v

## build: 打包当前平台的生产应用并内置仓库内源规则，产物在 apps/desktop/build/
build:
	@if [ ! -f "$(CURDIR)/apps/luvia-rules/kanju/rule.json" ]; then \
		echo "build: 警告：apps/luvia-rules 下没有可用的规则目录，打包后的应用会显示“没有可用数据源”"; \
	else \
		echo "build: 把 apps/luvia-rules 内置为 luvia-rules 资源；运行时 LUVIA_RULES_DIR 或用户数据目录 rules/ 优先"; \
	fi
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

## check: 提交前检查（格式、静态检查、测试、仓库规则、catalogs、Solar 图标）
check: vet test rules i18n icons-check
	@unformatted="$$(gofmt -l $(APP))"; \
	if [ -n "$$unformatted" ]; then echo "需要 gofmt: $$unformatted"; exit 1; fi

## rules: 检查仓库规则（临时产物、文档、命令、依赖方向、密钥、文案与配色字面量）
rules:
	@python3 -B scripts/check-rules.py

## rules-verify: 用规则自带 fixtures 离线验证 luvia-rules 的每个源（不访问真实上游）
rules-verify:
	@cd $(APP) && CGO_ENABLED=0 go test ./tests -run '^(TestKanju|TestFakeUpstream|TestLoaderLoadsCheckedInRules|TestRule|TestSelector|TestTemplate)' -count=1 -v

## i18n: 校验多语言 catalogs 的占位符与复数形式（译文完整性由 make rules 检查）
i18n:
	@cd $(APP) && $(MYGO) i18n check

## cover: 统计手写代码的语句覆盖率（生成代码不计），低于门槛就失败
cover:
	@dir="$$(mktemp -d)"; trap 'rm -rf "$$dir"' EXIT; \
	cd $(APP) && CGO_ENABLED=0 go test -coverpkg=./... -coverprofile="$$dir/cover.out" ./... > /dev/null && \
	python3 -B $(CURDIR)/scripts/check-coverage.py --profile "$$dir/cover.out" --min $(COVER_MIN)

## race: 用竞态检测器跑测试（较慢，CI 执行，不能设 CGO_ENABLED=0）
race:
	@cd $(APP) && CGO_ENABLED=1 go test -race ./...

## check-gen: 确认生成代码与 catalogs、Solar 母版一致（会重新生成，只在干净检出上执行）
check-gen:
	@python3 -B scripts/generate-desktop-icons.py
	@cd $(APP) && $(MYGO) generate > /dev/null
	@if ! git -C $(CURDIR) diff --quiet -- $(APP); then \
		echo "生成物与 catalogs、Solar 母版不同步：本地跑 go generate ./... 与 go tool mygo generate 后提交"; \
		git -C $(CURDIR) --no-pager diff --stat -- $(APP); \
		exit 1; \
	fi
	@echo "生成物与 catalogs、Solar 母版一致"

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
