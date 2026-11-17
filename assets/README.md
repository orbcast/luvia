# 共享素材

`assets/` 只存放项目正式使用、跨应用复用的素材母版，例如品牌图标、标志、插画、动效帧与界面 SVG。各端从统一母版生成所需资源，避免在多个应用中分别维护同一份源文件。这里不是 Agent 工作目录、下载目录或临时文件收集区。

## 禁止垃圾文件与运行时产物

禁止放入与正式交付无关的垃圾文件，以及 Agent 工作时产生的任务计划、任务服务文件、进度/状态文件、交接记录、一次性脚手架、探测/验证脚本、下载样本、参考截图、预览图、调试日志、缓存、测试报告和覆盖率产物。用户提供的设计参考图也不复制到本目录，设计结论记录在 [DESIGN.md](../DESIGN.md)。

上述临时内容必须在操作系统临时文件区域创建和执行，例如通过 `mktemp -d` 或 Python `tempfile.TemporaryDirectory()` 获取任务独立目录，不能把仓库中的 `tmp/`、隐藏目录或 `.gitignore` 当作替代。任务结束后清理自己创建的临时文件与进程，保留用户原文件。

正式 SVG 的来源/许可清单等必要元数据属于资源交付的一部分，可以随素材保存；用户要求交付的可复用工具与其测试放在所属脚本/技能目录，不混入 `assets/`。新增文件前确认它确实有正式用途与消费者，不以“之后可能有用”为由收存工作产物。

## 目录与格式

目前有 [icons/](icons/README.md) 与 [brand/opening/](brand/opening/README.md)：前者记录品牌图标母版、生成流程和平台差异，后者记录开屏动画母版与生成方式。界面图标按根目录 [AGENTS.md](../AGENTS.md) 使用 Solar，通过 [use-iconify-skill](../.agents/skills/use-iconify-skill/SKILL.md) 获取；真正使用时才建立 `ui-icons/solar/`，随 SVG 保留 `manifest.json` 来源、作者、许可与校验记录，并遵循 [DESIGN.md](../DESIGN.md) 的主题颜色。新增素材按用途放到 `assets/<name>/`，仅在确有素材时创建目录，并在对应 README 中写明用途、来源、授权、格式与生成方式。

图形源优先使用 SVG，栅格母版使用无损 WebP。应用业务代码放入 `apps/`，可复用模块放入 [packages/](../packages/README.md)，开发脚本放入 `scripts/`；测试夹具留在所属应用或模块的测试资源目录（如 Go 包的 `testdata/`），不要与品牌素材混放。

## 修改与生成

修改共享素材时只改母版，再通过明确的脚本生成应用所需格式，不手工修改生成物，也不把母版复制到各端分别维护。

当前图标在仓库根目录执行：

```sh
make icons
```

该命令调用 `scripts/icons.sh`，把图标同步到 `apps/desktop/resources/icon.png` 和 `apps/desktop/AppIcon.icon/Assets/mark.svg`。具体依赖与平台规则见 [图标说明](icons/README.md)。开屏动画母版在 [brand/opening/](brand/opening/README.md)，用 `make brand` 从图标母版重新生成，不手改生成物。其他素材应在引入时定义自己的生成流程，不能假定 `make icons` 会处理所有素材。

应用独有的配置、资源封装和平台生成格式放在相应 `apps/<name>/`。必要的生成资源随母版修改一起提交，使日常构建不依赖素材转换工具；构建产物、缓存和临时预览不放在本目录。

当前桌面应用的 macOS 图标由系统提供圆角遮罩和高光，Windows、Linux 与开发版使用既有方案。未来 Swift 等应用按自身资源工具链接入共享母版，不能假定当前脚本自动处理所有应用。修改素材后的验证遵循 [AGENTS.md](../AGENTS.md)，按实际视觉和平台风险检查，不默认执行完整 GUI 回归。
