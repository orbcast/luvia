# 红果短剧（hongguo）

`https://hongguoduanju.com/` 的网页端，供 Luvia 的桌面播放器读取片单、详情、选集与播放地址。

## 来源与覆盖

- 站点：`hongguoduanju.com`。除搜索外，页面是服务端渲染的 HTML，数据写在页面里的 `_ROUTER_DATA = {...}` 赋值中，所以 `home`、`list` 的分类分支、`detail`、`episodes` 都声明 `"response": "routerData"`，由引擎从 HTML 里取出这段 JSON 再交给选择器。
- 客户端头：`http.headers` 带桌面 Chrome 的 `User-Agent`。搜索接口 `/api/search/result` 不带浏览器 UA 时直接返回 `{"code":"search_rejected","message":"Search is temporarily unavailable."}`，其余页面不挑 UA；规则统一带同一个 UA，避免一个源出现两套身份。
- 首页：`/`，`loaderData.page.homeSections` 有四个 tab（`all` 热播短剧、`human` 热播真人剧、`comic` 热播漫剧、`ai` 热播AI剧），每 tab 九条，规则把 tab 名当分区标题。首页卡片不带 `vid_list`，选集只能在详情页取。
- 列表与搜索：两种形态用 `when` 二选一。不带关键词走 `/category?page=N` 的 HTML，`page=1` 会被 301 到 `/category`，引擎跟随重定向后仍能拿到同一页数据；带关键词走 `/api/search/result?query=&page=&count=10`，`count` 上限 10，`offset` 被忽略，翻页只能靠 `page`。两条分支的输出用 `|` 候选写法共用一套字段映射。
- 卡片标题：首页与搜索用 `series_title`，分类页只有 `series_name`，字段用 `coalesce` 取第一个非空。分类标签首页给 `category_list[*].name`（最多三项），分类页给 `tags` 字符串数组，搜索接口两项都不稳定，因此搜索结果的 `tags` 为空。
- 详情：`/detail?series_id={id}`，`loaderData.detail_page.seriesDetail` 给 `series_name`、`episode_cnt`、`accessible_episode_cnt`、`episode_right_text`、`tags`、`celebrities[*].nickname` 与全量 `vid_list`。
- 选集：`seriesDetail.vid_list` 是纯字符串的 vid 数组，上游不提供集号，规则用 `{"id": "item", "number": "index"}` 按位置编号，`index` 从 1 开始。
- 播放地址：`POST /api/video/protected/fallback-play-info`，请求体 `{"item_id": "<vid>"}`，返回 `main_url`（未加密的普通 mp4）与 `url_expire`。`url_expire` 是绝对 unix 秒、约二十分钟后过期，规则不声明 `expiresIn`，地址失效时由播放器重新解析。

## 已知限制

- 只有前 3~5 集能拿到播放地址（`accessible_episode_cnt`），第 6 集起上游返回 403 `{"error":"public_fallback_unavailable"}`，这是官方付费墙，不在覆盖范围内。该集数会随渠道浮动，界面按上游实际返回值展示。
- 搜索必须带桌面 UA，且 `count` 最大 10、`offset` 无效；分类页的 `sort_type`、`gender`、`tab`、`keyword` 参数在 301 后都被丢弃，不能用来筛选。
- 首页与分类页的卡片 `vid_list` 为空，选集必须进详情页，多一次请求。
- 播放地址没有扩展名，靠响应头的 `video/mp4` 判定；上游若改走 HLS 或加密，`urlKind` 与 `preferUrlKind` 要一起改。
- 上游若调整 `_ROUTER_DATA` 的键名或字段，`fixtures/` 与 `rule.json` 要一同更新，`make rules-verify` 会在离线用例里先暴露。

## 文件

- `rule.json`：规则本体。
- `fixtures/`：`home.page.txt`、`category.page.txt`、`detail.page.txt` 三个内嵌 `_ROUTER_DATA` 的页面样例（含同名标记的干扰项），`search.json`、`play.json`、`play-locked.json` 三个接口样例，覆盖正常、锁定集与缺字段等边界，`make rules-verify` 用它们离线验证。
