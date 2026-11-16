# 0005 多语言 catalogs 与文案测试

- 状态：已采纳
- 日期：2026-11-16

## 背景

桌面示例最初把中文直接写在界面代码里，同时又用中文标签定位测试控件。等到真正接入 MyGo 多语言时，界面文案、测试查找和语言切换会互相牵制：文案换了，测试跟着碎；测试写死中文，界面就不能翻译。项目也需要一个明确口径回答"品牌名要不要翻译""新增一句文案要改哪些文件"。

## 决定

用户可见文案放 `apps/desktop/locales/`：`en.json` 是源语言并带 `@key` 说明与占位符类型，`zh-Hans.json` 提供中文；`apps/desktop/mygo.json` 的 `i18n` 段指向该目录。`go tool mygo generate` 由 catalogs 生成 `locales/catalog_gen.go` 的类型化函数，界面只调用生成函数，不写字面量；改动 catalogs 后必须重新生成并提交。

文案中的产品名 `Luvia`（窗口标题）不翻译，作为品牌标识保留字面量。

测试按语言取值：默认用 `locales.In("en")` 拿查找用的标签与断言文本，因此改中文译文不会碰坏测试；需要验证"界面跟随语言"时用 `i18n.SetLocaleFunc(func() string { return "zh-Hans" })` 并在 `t.Cleanup` 里恢复。新增文案的顺序固定为：改 `en.json`（必填）→ 补其他语言 → `make i18n` → `go tool mygo generate` → 用生成的函数替换字面量。

## 后果

界面代码里不再出现中日韩文字符串，这一条由 `scripts/check-rules.py` 检查（测试文件与注释除外），规则不再只是口头约定。代价是每次新增一句文案都要动 catalogs 并重新生成，小改动看起来更重；换来的是占位符、复数形式和译文完整性由工具校验，翻译缺失会在 `make check` 里暴露。语言切换与持久化交给 MyGo 的应用级 API；桌面框架接入自定义菜单后，通过 OnLocaleChanged 重建菜单以同步标签，见 [ADR 0007](0007-desktop-shell.md)。
