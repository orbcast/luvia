# Solar 导航图标

本目录保存桌面应用实际使用的 Solar SVG 母版（Linear 与 Bold），以及 Iconify 技能脚本生成的 `manifest.json`。图标由 **480 Design** 创作，使用 **CC BY 4.0** 许可；来源为 [Solar](https://www.figma.com/community/file/1166831539721848736)，许可全文见 [Creative Commons Attribution 4.0](https://creativecommons.org/licenses/by/4.0/)。每个图标的完整标识、原始 URL、作者、许可和 SHA-256 均记录在清单中。

当前使用导航的五个 Linear 母版 `solar:home-linear`、`solar:clapperboard-linear`、`solar:heart-linear`、`solar:magnifer-linear`、`solar:settings-linear` 与其对应的五个 Bold 选中态 `solar:home-bold`、`solar:clapperboard-bold`、`solar:heart-bold`、`solar:magnifer-bold`、`solar:settings-bold`，以及发现页的 `solar:play-bold`（主播放操作）、`solar:info-circle-linear`、`solar:bookmark-linear`、`solar:alt-arrow-right-linear`、选择控件用的 `solar:alt-arrow-down-linear`；播放页控制条另用 `solar:close-linear`（关闭播放）、`solar:rewind-10-seconds-back-linear` 与 `solar:rewind-10-seconds-forward-linear`（后退/快进 10 秒）。导航选中项用 Bold，未选中用 Linear。SVG 保留下载原始字节与 `currentColor`，不手动修改、格式化或按主题复制。

从仓库根目录执行 `make icons-check` 离线校验来源、哈希和项目图标策略；执行 `python3 -B scripts/generate-desktop-icons.py` 生成桌面 `internal/icons/icons_gen.go`，执行 `make check-gen` 确认生成物同步。运行时仅使用本地嵌入资源，颜色由应用主题提供。获取或更换母版必须使用 [use-iconify-skill](../../../.agents/skills/use-iconify-skill/SKILL.md) 的脚本，品牌图标走独立的 [assets/icons](../../icons/README.md) 流程。
