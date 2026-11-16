# Solar 导航图标

本目录保存桌面应用实际使用的五个 Solar Linear SVG 母版，以及 Iconify 技能脚本生成的 `manifest.json`。图标由 **480 Design** 创作，使用 **CC BY 4.0** 许可；来源为 [Solar](https://www.figma.com/community/file/1166831539721848736)，许可全文见 [Creative Commons Attribution 4.0](https://creativecommons.org/licenses/by/4.0/)。每个图标的完整标识、原始 URL、作者、许可和 SHA-256 均记录在清单中。

当前使用 `solar:home-linear`、`solar:clapperboard-linear`、`solar:heart-linear`、`solar:magnifer-linear`、`solar:tuning-linear`。SVG 保留下载原始字节与 `currentColor`，不手动修改、格式化或按主题复制。

从仓库根目录执行 `make icons-check` 离线校验来源、哈希和项目图标策略；执行 `python3 -B scripts/generate-desktop-icons.py` 生成桌面 `internal/appshell/icons_gen.go`，执行 `make check-gen` 确认生成物同步。运行时仅使用本地嵌入资源，颜色由应用主题提供。获取或更换母版必须使用 [use-iconify-skill](../../../.agents/skills/use-iconify-skill/SKILL.md) 的脚本，品牌图标走独立的 [assets/icons](../../icons/README.md) 流程。
