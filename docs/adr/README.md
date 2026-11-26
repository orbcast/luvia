# 架构决策记录（ADR）

`docs/adr/` 保存已经定下的工程决策：为什么这么做、放弃了什么、以后怎么改。它回答"为什么"，不重复"怎么做"——具体步骤归所属应用的 README、`AGENTS.md` 或操作指南，这里只链接。

## 什么时候写

决策会影响多个应用、模块边界、构建与发布流程、用户可见行为，或者以后有人会问"为什么不是更方便的那种做法"时，写一篇。局部实现细节、随手可改的参数、临时试验不写。已经被推翻的决策不删，改状态并在"后果"里写清取代关系。

## 写法

文件名 `NNNN-kebab-case.md`，编号四位、递增、不复用。正文保持短：背景说约束和当时的事实，决定说选了什么和为什么是它，后果同时写代价和触发重新评估的条件。模板见 `.agents/skills/grill-with-docs/ADR-FORMAT.md`；本仓库只要其中的背景、决定、后果三节加状态与日期，不为了格式完整硬填章节。

## 索引

- [0001 应用与共享模块分离](0001-apps-and-packages.md)
- [0002 分层 Agent 规则与自维护预算](0002-layered-agent-rules.md)
- [0003 MyGo CLI 由 go tool 固定](0003-go-tool-pinned-cli.md)
- [0004 界面图标与品牌图标分离](0004-interface-and-brand-icons.md)
- [0005 多语言 catalogs 与文案测试](0005-localized-messages.md)
- [0006 验证闸门：make check、pre-commit 与 CI](0006-verification-gates.md)
- [0007 桌面框架与应用内职责分层](0007-desktop-shell.md)
- [0008 源规则用声明式 DSL，desktop 只做通用解释](0008-rule-dsl-and-engine.md)
- [0009 发现页由源规则首页与本地观看库组合](0009-home-page-and-local-library.md)
- [0010 视频经本地缓存代理播放](0010-video-cache-proxy.md)
- [0011 超分与杜比止步于系统播放层](0011-playback-layer-boundary.md)
- [0012 图片缓存按字节预算淘汰](0012-artwork-cache-budget.md)
- [0013 播放页在一条线路不出画面时换下一条](0013-playback-line-fallback.md)
- [0014 让源规则同时吃 JSON 接口与服务端渲染页面](0014-html-pages-as-rule-sources.md)
- [0015 弹幕按标题在站外匹配，规则只提供剧集](0015-danmaku-from-outside-the-rule.md)
- [0016 macOS 使用系统原生红黄绿按钮](0016-native-window-buttons-on-macos.md)
- [0017 源自带时间轴弹幕时站外匹配让位](0017-upstream-danmaku-wins.md)
- [0018 弹幕层自己走播放头，不等播放器的报告](0018-danmaku-walks-the-playhead-between-reports.md)
- [0019 访问密钥在应用内填写](0019-access-keys-in-the-app.md)

新增决策时在本列表补一行，写完跑 `make rules` 确认索引、文件名和链接都在。
