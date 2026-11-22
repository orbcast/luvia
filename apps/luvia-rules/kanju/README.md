# 看剧（kanju）

`https://kanju1.com/` 的公开 JSON 接口，供 Luvia 的桌面播放器读取片单、详情、选集与播放地址。

## 来源与覆盖

- 站点：`kanju1.com`，接口带 HMAC-SHA256 请求签名（`x-ai-movie-*` 头），签名消息为 `METHOD\n路径与查询\n时间戳\n随机数`；客户端头写在 `rule.json` 的 `http.headers` 里，签名密钥不落盘，`sign.secret` 用 `${env:KANJU_SECRET}` 从环境变量取。
- 密钥放在仓库根目录的 `.env`（`.gitignore` 挡住 `.env*`，只放行不含值的 `.env.example`），先 `cp .env.example .env` 再填 `KANJU_SECRET=...`，`make dev` 会自动加载；已 export 的同名变量优先于文件。变量缺失时引擎不再发请求，直接报 `unauthorized` 并在详情里点名缺失的变量，界面上就是「这个数据源加载失败」加这行说明；`${env:NAME}` 的替换规则见 [规则 DSL](../../../docs/rule-dsl.md)。
- 首页：`/v1/feed/home`，取 11 个分区、每区 8 张卡；发现页的 Hero 挑横幅最全的分区。
- 列表与搜索：`/v1/browse/catalog`，每页 24 条。不带关键词时按最新列表浏览；带关键词时通过 `searchIntent` 把 `intent` 换成 `search`——上游只在该取值下按关键词匹配，浏览时不发这个参数。
- 当日更新：`/v1/watch-calendar`，按周取某一天的更新单。上游每天每类只给 6 条预览（`limit` 再大也是 6），超过的只有 `more_url` 游标，本规则暂不跟进；周表按 `[by:weekday]` 挑出请求的那一天，整周一起返回时也不会串天。
- 详情：`/v1/catalog/{id}`；选集：`/v1/catalog/{id}/episodes`，按 offset 翻页，每页 200 集。
- 播放地址：`/v1/playback/resolve/{payload}` 给出线路数组，线路可能是直链，也可能要再用 `/v1/playback/resolve-line` 兑换票据；规则优先 mp4。
- 弹幕：`/v1/danmuku/episodes/{episodeId}/source` 说清单在哪，清单按 `segment_duration_ms`（默认 5 分钟）把整集切成窗口，每个窗口是 CDN 上的一个文件。注意拼写是 `danmuku`，与本仓库通用的 `danmaku` 不是同一个词。规则用 `manifest` + `segments` 两块描述这条链，播放页拿到的是秒级的 `source.Danmaku`，优先于站外匹配；细节见 [ADR 0017](../../../docs/adr/0017-upstream-danmaku-wins.md)。
- 上游的鉴权服务会短暂不可用：这时它回 `503 {"error":"project_auth_unavailable","retryable":true,"reason":"timeout"}`，有时干脆回一个空页。两种情况都当临时故障处理，`http.retryWhen` 把这两个响应点名成可重试（`{"status": 503, "contains": "project_auth_unavailable"}` 与 `{"status": 200, "empty": true}`），走与 5xx 同一份重试预算和退避，耗尽后才报 `network`。翻页中途遇到空页也会重试，不会把半截列表当成只有一页。
- 弹幕的两个历史坑：窗口文件名是 `.json.gz`、清单也写 `content_encoding:gzip`，但实际响应是明文 JSON 且不带 `Content-Encoding`，引擎按魔数判断、不认声明；窗口里的每条弹幕是数组 `[毫秒, 模式, 颜色, 字号, 文本, id]`，不是对象，所以 `segments.output` 里用数字下标选择器（`"4"` 是文本），毫秒用 `seconds` 换成秒。

## 已知限制

- 只有免费线路，付费/会员线路不在覆盖范围内。
- 弹幕清单里的 `comment_count` 与窗口 `count` 会随编辑变化（实测同一集 `revision:19`、205 条），数值只当提示，不当契约；窗口按 `from_ms`/`to_ms` 读，不按条数读。
- 搜索走的是上游的关键词匹配，不做拼音或模糊改写；空关键词会退化成浏览列表。
- 当日更新的 `note` 是上游原文（例如“更新至第 26 集”），播放器原样展示，不翻译。
- 上游若调整字段或签名头，`fixtures/` 与 `rule.json` 要一同更新，`make rules-verify` 会在离线用例里先暴露。

## 文件

- `rule.json`：规则本体。
- `fixtures/`：五个以上操作的响应样例，覆盖正常、空的一天与缺字段等边界，`make rules-verify` 用它们离线验证。弹幕三件套是 `danmuku-source.json`、`danmuku-manifest.json`、`danmuku-segment-1.json`/`danmuku-segment-2.json`（两个窗口，一个取第一条、一个取第二条）。
