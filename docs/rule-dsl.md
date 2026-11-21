# 源规则格式（Rule DSL）

本文件是 `apps/luvia-rules/` 与 `apps/desktop` 解析层之间的正式契约：规则用 JSON 描述一个上游站点的 HTTP 交互，desktop 的解释引擎按固定步骤词表执行，把它们映射成播放器需要的六个操作。规则是纯数据，不含脚本、不依赖宿主语言；引擎只实现词表内的行为，不为单个上游开后门。

## 分层

| 位置 | 职责 |
| --- | --- |
| `apps/luvia-rules/<source-id>/` | 一个上游一个目录：`rule.json`、`fixtures/`、`README.md`。目录名即源 ID |
| `apps/desktop/internal/source` | 契约：`Source` 接口、`Ref`、领域类型、错误分类，不依赖 HTTP / JSON |
| `apps/desktop/internal/source/rule` | 规则解码、默认值、结构校验 |
| `apps/desktop/internal/source/engine` | 解释器：请求、签名、翻页、字段映射、线路兑换 |
| `apps/desktop/internal/source/loader` | 扫描规则目录、隔离坏规则、收集诊断、热重载 |

依赖方向单向下行，`luvia-rules` 不引用 desktop 的任何 Go 类型，因此可以随时拆成独立仓库。

## 六个操作与能力声明

| 操作 | 方法 | 块的键 |
| --- | --- | --- |
| 首页推荐 | `Home(ctx)` | `home` |
| 列表与搜索 | `List(ctx, ListQuery{Keyword, Kind, Page})` | `list` |
| 当日更新 | `Updates(ctx, Day{Weekday, Kind})` | `updates` |
| 详情 | `Detail(ctx, Ref)` | `detail` |
| 选集 | `Episodes(ctx, Ref)` | `episodes` |
| 解析播放地址 | `Resolve(ctx, Episode, lineID)` | `resolve` |

块存在即代表支持，缺少该块时调用返回 `unsupported` 分类的错误。没有隐式的兜底逻辑，例如 `list` 不会自动复用 `home` 的规则。

同一个上游的多个播放源、多条线路属于运行时数据（响应里的线路数组），不拆成多条规则；一个上游只对应一个目录。

## 文件布局

```
apps/luvia-rules/
  <source-id>/
    rule.json      # 规则本体，id 必须与目录名一致
    fixtures/      # 各操作的响应样例，供离线验证
    README.md      # 来源、授权、覆盖范围、已知限制
```

规则目录在运行时的解析顺序：环境变量 `LUVIA_RULES_DIR` → 用户数据目录下的 `rules/`（存在规则时）→ 应用包内 `Resources/luvia-rules/`（`mygo.json` 的 `resources` 在打包时从 `apps/luvia-rules` 复制）。desktop 源码不出现任何上游名称；开发态用环境变量指向 `apps/luvia-rules`。

## rule.json 结构

```json
{
  "schemaVersion": 1,
  "id": "example",
  "name": "示例源",
  "baseURL": "https://example.test",
  "http": {"timeoutMs": 30000, "retries": 2, "concurrency": 4, "maxPages": 20,
           "headers": [{"name": "Accept", "value": "application/json"}]},
  "sign": {"secret": "...", "message": "{method}\n{path}\n{timestamp}\n{nonce}",
           "timestampUnit": "ms", "nonceBytes": 16,
           "headers": [{"name": "x-signature", "value": "{signature}"}]},
  "home": {"steps": [], "output": {}},
  "list": {"kindValues": {"movie": "1"}, "searchIntent": "search", "steps": [], "output": {}},
  "updates": {"kindValues": {"series": "series"}, "days": "weekday", "steps": [], "output": {}},
  "detail": {"steps": [], "output": {}},
  "episodes": {"steps": [], "output": {}},
  "resolve": {"steps": [], "lines": {}}
}
```

顶层键只有这些；`schemaVersion` 必填且当前为 `1`，未知键导致整条规则被拒绝（严格解码）。`id` 必须等于目录名。`baseURL` 必须是 http/https、带主机、无 query 与 fragment。

`http` 与 `sign` 可选。`http` 缺省值：超时 30 秒、重试 2 次、并发上限 4、翻页上限 20。`timeoutMs` 是单次尝试的上限；重试只对幂等方法（GET / HEAD）在传输错误与 5xx 时进行，退避 200 ms × 2ⁿ。

“sign” 存在时，除显式写 “sign”: false 的步骤外，所有请求都带签名头：{method}、{path}（含 query）、{timestamp}、{nonce} 四个变量供 message 模板使用，{signature} 供头模板使用。timestampUnit 取 ms 或 s；nonceBytes 是随机字节数，十六进制编码后使用。sign.secret 支持 `${env:NAME}`，取值在发请求前从环境变量替换；变量缺失时不发送请求，直接按 unauthorized 失败（Detail 里带缺失的变量名），不拿占位符去签名。

`request` 对象（步骤里的与线路兑换里的都算）可以声明 `response`，说明这次响应的形态：缺省与 `"json"` 都按 JSON 解析 body；`"routerData"` 用于服务端渲染的页面——页面把同一份数据塞在脚本的 `_ROUTER_DATA = {...}` 赋值里下发，引擎按这个字面量定位，要求其后（允许空白）确实是 `=` 再跟 `{`，否则跳过——页面别处提到这个名字、或者只是读一下全局，都不能被当成载荷；从那个 `{` 起逐字节配平（跳过字符串字面量与转义，标题里的花括号不会提前结束对象），配平出的对象还要通过 JSON 校验才采用，否则继续找下一次出现。这样同一个源既能读 JSON 接口，也能读分类页，六个操作不必因为响应形态不同拆成两个源；决定与代价见 [ADR 0014](adr/0014-html-pages-as-rule-sources.md)。取值只有 `""`、`"json"`、`"routerData"` 三种，写错时加载规则即报 `unsupported response`。

## 步骤（steps）

每个块有一个 `steps` 数组，按顺序执行，`as` 把该步的响应文档绑定成命名变量；`request` 与 `paginate` 两种操作。

`request`：

```json
{"op": "request", "as": "res", "method": "GET", "path": "/v1/catalog/{id}",
 "query": [{"key": "limit", "value": "24"},
           {"key": "intent", "value": "search", "omitWhen": {"template": "{keyword}"}}],
 "headers": [{"name": "x-locale", "value": "{kind}"}],
 "body": {"ticket": {"template": "{ticket}"}},
 "sign": false}
```

`path` 必填且以 `/` 开头；`method` 缺省 GET；body 按键排序后输出 JSON 并自动带 `Content-Type: application/json`。

query 的 `value` 是模板，常量也要写成普通文本（`"24"`、`"latest_catalog"`），只有模板里解析不出的洞或空值才会丢掉这个参数，所以上游把空关键字当作“不过滤”时，那个参数照样会发出去。需要“这次不发”的参数用 `omitWhen` 声明：它求值为真时跳过该参数；同名参数写两条、用相反的 `omitWhen`，就能在同一次调用里二选一。

`paginate`：在同一步内翻页，并把每页 `items` 合并成一个数组变量。

```json
{"op": "paginate", "as": "pages", "mode": "offset", "size": 200, "maxPages": 20,
 "path": "/v1/catalog/{id}/episodes",
 "query": [{"key": "limit", "value": "{size}"}, {"key": "offset", "value": "{offset}"}],
 "items": "episodes[*]", "hasMore": "episode_pagination.has_more"}
```

`mode` 为 `page` 或 `offset`。两种模式下都会提供 `{page}` 与 `{offset}`：`page` 模式 `page = start + i`、`offset = i × size`；`offset` 模式反之。`items` 必填，选择器以「单页响应」为根；`hasMore` 可选，解析为 false 即停止；`items` 为空或达到 `maxPages` 也停止。

步骤还可以带 `when`：它是一个值（选择器、函数、模板或字面量），求值为真才执行这一步，缺省则总执行。它是“一次调用里二选一”的手段——有搜索词走搜索接口、没有搜索词走分类页。求值为假、或求值本身失败时，这一步不绑定任何变量，后续步骤与输出看到的是变量缺失而不是空值：模板里的洞解析不出会让整个模板失败，`from` 则继续试下一个候选。

变量作用域：块内所有已绑定的 `as` 名字可被后续步骤与输出引用；`paginate` 绑定的是合并后的数组。同一块内不允许重复绑定同一个名字，例外是带 `when` 的步骤：它是二选一里的一个分支，可以和另一个分支共用一个 `as`，因为同一次调用里只会有一个真的执行。

## 当日更新（updates）

`updates` 把某一天的更新单映射成卡片，播放器据它画首页的周表；缺这个块时周表只用本地追剧记录。

```json
"updates": {
  "kindValues": {"series": "series", "anime": "anime"},
  "days": "weekday",
  "steps": [{"op": "request", "as": "res", "path": "/v1/watch-calendar",
             "query": [{"key": "weekday", "value": "{weekday}"},
                       {"key": "kind", "value": "all"}]}],
  "output": {"from": "res.days[*]", "cards": {"from": "res.days[by:weekday].groups.series.preview[*]",
    "fields": {"id": "id", "title": "title", "note": "latest_episode_label",
               "posterUrl": "poster_url", "kind": {"fn": "kind", "args": ["content_kind"]}}}}
}
```

- 内建变量 `weekday`（周一为 1，周一至周日 1–7）与 `kind`（经 `kindValues` 映射，未列出的分类不发送）。
- 卡片字段是列表卡片的全部字段再加 `note`，用来说明当天为什么在这里（如“更新至第 26 集”），由上游文案直出，不做翻译。
- `days` 指定这一次调用取哪一天：上游一次只答一天时，请求参数已经限定了日期，`days` 可以写成任意可解析的选择器（例如 `"weekday"`）而不影响结果；上游一次返回整周时，用 `days` 比较每一天的字段，也可以用 `[by:字段]` 在映射里按字段挑出那一天。
- 卡片选择器的 `from` 允许在中间出现 `[*]`：它逐级展开并合并成一层列表，`res.days[*].groups.series.preview[*]` 即“所有天的所有预览”。`[字段名]` 取调用方给的位置（例如 `res.days[weekday]`），`[by:字段]` 按字段值匹配数组中的一项（例如 `res.days[by:weekday]`）；两者都只能读播放器传进来的内建变量。
- 首页周表的每天一次请求，播放器会缓存当天结果；某一天失败只影响那一天。

## 选择器、模板与函数

选择器是受限的点路径：`a.b`、`res.cards[0].title`、`res.cards[*]`。`[n]` 取下标，`[*]` 展开成数组，根名是变量名（内建变量见下）。`[*]` 在最后一段时就是该数组本身；出现在中间时逐级展开并合并成一层，`res.days[*].groups.preview[*]` 是所有天合并后的预览列表。`[字段名]` 取调用方传入的位置（如 `res.days[weekday]`），`[by:字段]` 按字段值匹配数组中的一项（如 `res.days[by:weekday]`），两者都只读播放器传入的内建变量，其他情况加载规则时报错。

模板是带花括号的插值串：`/v1/playback/resolve/{payload}`、`{method}\n{path}`。任一洞解析失败，整个模板失败。

值有四种形态：

| 形态 | 写法 | 用途 |
| --- | --- | --- |
| 选择器 | `"cards"` 或 `"res.cards[*]"` | 取字段 |
| 函数 | `{"fn": "coalesce", "args": ["label", "display_label", "id"]}` | 变换与兜底 |
| 模板 | `{"template": "{id}"}` | 拼接字符串 |
| 字面量 | `{"literal": "movie"}` | 与同名字段区分 |

注意裸字符串一律按选择器解析，因此需要插值时必须写 `{"template": "..."}`，不能写成 `"{page}"`。

可用函数（闭集）：

| 函数 | 参数 | 行为 |
| --- | --- | --- |
| `coalesce` | ≥1 | 返回第一个可解析且非空的值 |
| `trimPrefix` / `trimSuffix` | 2 | 去前缀 / 后缀 |
| `trim` | 1 | 去空白 |
| `lower` / `upper` | 1 | 大小写 |
| `int` | 1 | 转整数，失败则不解析 |
| `kind` | 1 | 折叠成 `movie` / `series` / `anime` / `variety`，无法识别归 `series` |
| `urlKind` | 1 | 推断为 `mp4` 或 `hls`（含 `m3u8` 或 `.m3u8` 结尾视为 hls） |
| `list` | 1 | 包成数组 |
| `first` / `last` | 1 | 取数组首尾 |
| `empty` | 1 | 值缺失、空数组、空字符串为真，其余为假 |
| `not` | 1 | 布尔取反，参数取不到时不解析 |

`kind` 的识别词：movie / film / documentary / doc → `movie`；anime / animation / donghua / cartoon → `anime`；variety / sports / reality / show → `variety`；series / tv / drama / short_drama / shortdrama → `series`。上游的细分标签保留在 `Tags` 里。

## 输出映射

`output` 用 `from` 指向一个变量（`list` 页也用它，通常是 `"res"`），再声明各字段的取值：

```json
"output": {
  "from": "res.cards[*]",
  "fields": {"id": "id", "title": "title", "kind": {"fn": "kind", "args": ["content_kind"]},
             "year": "year", "area": "area", "tags": "genres",
             "posterUrl": "poster_url", "backdropUrl": "backdrop_url", "remarks": "remarks"}
}
```

可用的字段名（其余视为规则错误）：

| 位置 | 允许的字段 |
| --- | --- |
| 卡片 / 列表项 | `id`、`title`、`kind`、`year`、`area`、`tags`、`remarks`、`overview`、`posterUrl`、`backdropUrl` |
| 详情 | 卡片字段 + `overview`、`actors`、`directors`、`episodeCount` |
| 选集 | `id`、`number`、`title`、`payload` |
| 首页分区 | `id`、`title`、`subtitle`（分区必须另有 `cards`） |

`cards` 描述分区内卡片的来源与字段。`page` 与 `hasMore` 仅用于列表：`{"from": "res.cards[*]", "page": "res.pagination.page", "hasMore": "res.pagination.has_more", "cards": {...}}`；列表页不允许写 `fields`。

`output.from` 可以写多个候选，用 `|` 隔开，例如 `"res.search.items[*]|res.category.items[*]"`：引擎按顺序取第一个能解析出结果的候选，全部失败才报 `"..." is missing from the response`（`invalid_response`）。它是给带 `when` 的二选一用的——两个分支绑同一个 `as`，`from` 列出各自响应里数组的位置，哪条真的跑了就读哪条。校验阶段每个候选都要单独站得住：能解析成选择器、根名在作用域里，候选之间不许出现空串，这样一个分支里的笔误不会被另一个分支盖住。同一套候选写法也适用于 `cards.from` 与 `resolve.lines.from`，它们各自从自己的根对象往下解析。

首页输出是分区的数组：

```json
"output": {"from": "res.sections[*]",
  "fields": {"id": "id", "title": "title", "subtitle": "subtitle"},
  "cards": {"from": "cards[*]", "fields": {"id": "id", "title": "title"}}}
```

详情与选集必须提供 `title`，并且不允许 `cards`、`page`、`hasMore`。选集缺少 `id` 的项被跳过，`payload` 是不透明字符串，用于把解析所需 token 带到 `resolve`。

映射走过的每一项除了自己的字段，还能读到两个名字：`item` 是这一项本身，`index` 是它在列表里的位置（从 1 开始）。列表项是纯字符串时（详情页只给一串 vid 就是这样）这是唯一能读的东西，`{"id": "item", "number": "index"}` 让选集拿到 id 与集号；对象项的行为不变，字段查找先于这两个名字，只有字段不存在时才落到 `item` / `index`。

首页、列表、详情缺 `id` 的卡片会被跳过。

## 线路与播放地址

`resolve` 块除 `steps` 外还有 `lines`：

```json
"resolve": {
  "steps": [{"op": "request", "as": "res", "path": "/v1/playback/resolve/{payload}"}],
  "lines": {
    "from": "res.line_options[*]",
    "fields": {"id": {"fn": "coalesce", "args": ["playback_source_id", "id"]},
               "label": {"fn": "coalesce", "args": ["label", "display_label", "provider_name"]},
               "urlKind": "url_kind", "url": "url", "ticket": "resolve_ticket",
               "default": "selected"},
    "exchange": {"ticket": "ticket",
                 "request": {"method": "POST", "path": "/v1/playback/resolve-line",
                             "query": [{"key": "view", "value": "compact"}],
                             "body": {"ticket": {"template": "{ticket}"}}},
                 "url": "line.url", "urlKind": "line.url_kind"},
    "expiresIn": "res.expires_in",
    "maxAttempts": 12,
    "preferUrlKind": "mp4"
  }
}
```

- `fields` 允许 `id`、`label`、`urlKind`、`url`、`ticket`、`default`，必须提供 `id` 与 `url`；重复 `id` 只保留首个。
- `url` 是绝对 http(s) 地址时直接返回；否则按 `ticket` 做二次兑换。`ticket` 优先取字段值，其次解析 `resolve://<ticket>` 形式。没有 `exchange` 或拿不到 ticket 时该线路不可用。
- 尝试顺序：指定线路 ID → 声明 `default` 的线路 → 声明顺序，最多 `maxAttempts` 条。`preferUrlKind` 非空时优先返回该类型的线路，若都没有则回退首个可用线路。
- `expiresIn` 以秒为单位输出到 `Playback.ExpiresIn`，由调用方决定何时重新解析；引擎不缓存直链，也不自动续期。

全部线路都失败时返回 `no_playable_line`，诊断里带每条线路的失败原因。

## 内建变量

| 块 | 变量 |
| --- | --- |
| `home` | 无 |
| `list` | `page`、`keyword`、`kind`（经 `kindValues` 映射后的上游取值） |
| `detail` | `id` |
| `episodes` | `id` |
| `resolve` | `id`、`payload`、`media`、`lineId` |
| `paginate` 步骤 | 额外 `page`、`offset`、`size` |

引用未声明的变量是规则错误，加载时即被拒绝，不会拖到运行时。

`list.kindValues` 把播放器的分类折叠成该站点的取值，例如 `{"movie": "1", "series": "2"}`；未列出的分类不发送 `kind`。

`list.searchIntent` 声明该站点匹配关键词时要求的 `intent` 之类的取值：填了它，带关键词的列表请求把同名查询参数换成这个值，不带关键词的列表请求则完全不发该参数；不填则查询原样发送。一个上游只有一个列表模式时不需要它。

`${env:NAME}` 会在请求头、密钥与常量位置替换成环境变量，缺失时保留原文。

## 执行与错误

每个源最多 4 个并发请求（可配置），请求受块级超时与 `ctx` 约束，取消优先于重试。响应超过 8 MiB 视为无效响应。

声明 `"response": "routerData"` 的请求，若整页都找不到能配平成合法 JSON 的 `_ROUTER_DATA` 对象，按 `invalid_response` 失败，诊断写 "response carries no router data"；对象抽出来了但内容不是规则以为的结构，错误会出现在后续选择器上（`from` 的候选全空、字段取不到），不在抽取这一步拦下。`when` 为假而跳过的步骤不会产生请求，也就没有错误可报。

错误带分类码、源 ID、出错的块与步骤，上游文案只进诊断字段，界面按分类码取本地化文案：

| 分类 | 含义 |
| --- | --- |
| `network` | 传输失败或 5xx |
| `timeout` | 超过单次或整体时限 |
| `canceled` | 调用方取消 |
| `unauthorized` | 401 / 403，通常是密钥或签名过期 |
| `not_found` | 404，或请求的线路不存在 |
| `rate_limited` | 429 |
| `invalid_response` | 状态码异常、非 JSON、响应过大 |
| `invalid_rule` | 规则本身不合法（加载阶段与运行期均可能出现） |
| `no_playable_line` | 所有线路都拿不到可播放地址 |
| `unsupported` | 该源缺少对应的块 |

引擎不做缓存、不做跨源聚合：首页合并、去重、降级与元数据缓存都在调用方。

## 新增一个源

1. 建 `apps/luvia-rules/<source-id>/`，目录名即 `id`。
2. 用浏览器开发者工具记录五个操作的请求与响应，确认签名、header、分页与线路形态。
3. 按本文档写 `rule.json`，响应样例存进 `fixtures/`。
4. 写 `README.md`：来源站点、覆盖范围、授权、已知限制（例如只覆盖免费线路）。
5. `make rules-verify` 应给出该源全部操作通过的结果；坏规则会被 loader 跳过并在诊断里说明原因，不阻塞其他源。

规则行为必须有可重复验证：`fixtures/` 要覆盖正常、空结果与缺字段等边界，不以真实网络是否在线作为唯一证据。真实网络的冒烟测试只作为手动检查。
