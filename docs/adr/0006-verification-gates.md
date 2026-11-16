# 0006 验证闸门：make check、pre-commit 与 CI

- 状态：已采纳
- 日期：2026-11-16

## 背景

规则写在 Markdown 里只能说清意图，不能阻止提交。此前仓库里唯一"会失败"的入口是 `make check`（vet + test + gofmt），Agent 完全可以在提交前跳过它；文档断链、索引漏项、临时文件入库、把中文写回界面代码、在控件里写十六进制颜色，都能悄悄进版本库并长期潜伏。

## 决定

把检查做成一条可重复执行的入口，三处共用：`make check`（`vet`、`test`、`rules`、`i18n`、`gofmt`）、`.githooks/pre-commit`（`make -s check`，用 `make hooks` 设为 `core.hooksPath`）、`.github/workflows/ci.yml`（文档规则在 Linux 上跑，应用检查在 macOS 上跑）。

`scripts/check-rules.py` 承担那些没有专用工具的判断：版本库不含构建产物、日志与临时目录；Markdown 相对链接可达；`docs/README.md` 与 `docs/adr/README.md` 覆盖目录内文档；ADR 文件名与编号合规；`AGENTS.md` 体量在预算内；`apps/` 下非测试 Go 代码没有中日韩文字面量；除集中 Token 文件外没有十六进制颜色；各语言 catalogs 覆盖源语言的每个键。图标另走 `make icons-check`，只在 `assets/ui-icons/solar/` 存在时校验。

## 后果

提交前会多花几秒，这是有意付出的成本；被拦下的问题必须在代码或脚本里解决，不通过 `--no-verify`、删除断言或放宽检查绕过。检查脚本只做只读判断、不自动修复，避免"自动格式化"掩盖真实问题。它覆盖不了原生渲染、平台行为、签名与公证，也不能代替人工评审；CI 配置尚未在远端真正跑过（仓库还没有 remote），这一点在 `README.md` 与提交说明里保持如实。新增可脚本化的规则时，优先加进脚本而不是再加一句散文。
