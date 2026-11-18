# 看剧（kanju）

`https://kanju1.com/` 的公开 JSON 接口，供 Luvia 的桌面播放器读取片单、详情、选集与播放地址。

## 来源与覆盖

- 站点：`kanju1.com`，接口带 HMAC-SHA256 请求签名（`x-ai-movie-*` 头），签名消息为 `METHOD\n路径与查询\n时间戳\n随机数`，密钥与客户端头写在 `rule.json` 的 `sign` / `http.headers` 里。
- 首页：`/v1/feed/home`，取 11 个分区、每区 8 张卡；发现页的 Hero 挑横幅最全的分区。
- 列表与搜索：`/v1/browse/catalog`，每页 24 条。不带关键词时按最新列表浏览；带关键词时通过 `searchIntent` 把 `intent` 换成 `search`——上游只在该取值下按关键词匹配，浏览时不发这个参数。
- 当日更新：`/v1/watch-calendar`，按周取某一天的更新单。上游每天每类只给 6 条预览（`limit` 再大也是 6），超过的只有 `more_url` 游标，本规则暂不跟进；周表按 `[by:weekday]` 挑出请求的那一天，整周一起返回时也不会串天。
- 详情：`/v1/catalog/{id}`；选集：`/v1/catalog/{id}/episodes`，按 offset 翻页，每页 200 集。
- 播放地址：`/v1/playback/resolve/{payload}` 给出线路数组，线路可能是直链，也可能要再用 `/v1/playback/resolve-line` 兑换票据；规则优先 mp4。

## 已知限制

- 只有免费线路，付费/会员线路不在覆盖范围内。
- 搜索走的是上游的关键词匹配，不做拼音或模糊改写；空关键词会退化成浏览列表。
- 当日更新的 `note` 是上游原文（例如“更新至第 26 集”），播放器原样展示，不翻译。
- 上游若调整字段或签名头，`fixtures/` 与 `rule.json` 要一同更新，`make rules-verify` 会在离线用例里先暴露。

## 文件

- `rule.json`：规则本体。
- `fixtures/`：五个以上操作的响应样例，覆盖正常、空的一天与缺字段等边界，`make rules-verify` 用它们离线验证。
