# 0004 界面图标与品牌图标分离

- 状态：已采纳
- 日期：2026-11-16

## 背景

仓库里同时存在两类"图标"，它们的来源、格式和生命周期完全不同：界面上可点击、可导航的操作与状态图标，和代表应用本身的品牌图标。把两者合并成一套流程会同时违反两条硬约束——界面图标必须能用语义前景色着色、随深浅主题变化，而品牌图标要在系统图标栈里被 `actool` 编译成 `Assets.car` 与 `.icns`，由系统加遮罩和高光。

## 决定

界面操作、导航与状态图标只使用 Solar SVG（默认 Linear，选中或强调可用 Bold），只通过 `.agents/skills/use-iconify-skill/scripts/iconify.py` 搜索与下载，交付前跑离线 `check`；SVG 保留 `currentColor` 与来源、许可信息，落在 `assets/ui-icons/solar/`，由各端从共享 SVG 生成本地资源，不按主题存两份路径。

品牌与应用图标走另一条链路：母版在 `assets/icons/`（矢量 `mark.svg` 加无损 WebP 栅格），`scripts/icons.sh` 把母版同步进 Icon Composer 文档与 `resources/icon.png`，构建期由 `actool` 生成平台格式。两者的来源、许可、生成命令分别记在 `assets/README.md`、`assets/icons/README.md` 和根 `README.md` 的平台说明里，互不替代。

## 后果

两套流程各自保持单一职责，但需要在文档里明确"哪种图标放哪里"，否则会被误当作重复内容合并掉。按主题维护两套彩色 SVG 的捷径被排除，界面图标只能靠 `currentColor` 适配；品牌图标也因此不做深浅两套，深浅差异交给系统。使用 Solar 图稿需要落实 CC BY 4.0 的署名要求。当前尚未引入任何界面图标，`assets/ui-icons/solar/` 不存在时 `make icons-check` 会明确跳过，不创建空目录。
