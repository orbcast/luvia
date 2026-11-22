# 0017 源自己带时间轴弹幕时，站外匹配让位

- 状态：已采纳
- 日期：2026-11-22

## 背景

[0015](0015-danmaku-from-outside-the-rule.md) 把弹幕层的来源放到规则外面：站外按标题匹配，规则只提供剧集。这对红果成立，因为它的官方弹幕接口需要整套签名与设备指纹。但把它当成唯一路径就不成立了——kanju 上游本来就托管弹幕，只是没人读。

kanju 的弹幕不在 `comments` 块里。`/v1/comments?variant_id=…` 是短评与剧评，`position_seconds` 大量为 0；真正的弹幕是另一套资产协议，拼写是 `danmuku`：

```
GET /v1/danmuku/episodes/{episodeId}/source   (需签名)
  → {"protocol":"aimovie.danmu-source.v1","status":"ready","manifestUrl":"https://lf5.wikjdd.cn/…/manifests/19.….json"}
GET {manifestUrl}
  → {"schema_version":"danmuku.manifest.v1","timebase":"milliseconds","comment_count":205,
     "segments":[{"from_ms":0,"to_ms":300000,"count":60,"url":"../segments/0-300000.….json.gz","content_encoding":"gzip"},…]}
GET {resolve(manifestUrl, segment.url)}
  → {"schema_version":"danmuku.segment.v1","items":[[0,1,14463824,25,"左上角笑死我了!!","dmc_…"],…]}
```

实测这一集 205 条、11 个窗口。两个必须按事实而非声明处理的点：segment 文件名叫 `.json.gz`、manifest 也写 `content_encoding:"gzip"`，但响应是明文 JSON 且不带 `Content-Encoding`；item 是紧凑数组，`[毫秒, 模式, 颜色, 字号, 文本, id]`，不是对象。

用户此前看到的是这套协议没接通时的退路：站外搜到同名但 0 条弹幕的投稿 → `ReasonEmpty` → 回落到 213 条几乎全部 `at=0` 的剧评 → 播放页把它们按 `i%8` 车道堆在原地。界面读起来像坏了，实际是两条路都没有好数据。

## 决定

规则 DSL 加一对可选块：`manifest` 说清单地址，`segments` 逐窗口读。它们必须成对出现——缺一半是规则错误，因为单独的 `manifest` 是一次没人读的回答，单独的 `segments` 没有窗口可读。

`manifest` 的 `output.fields` 只有一个字段 `source`；`segments` 复用 `comments` 的字段集，但多了 `list`：

```json
"segments": {
  "steps": [],
  "output": {
    "from": "document.items[*]",
    "fields": {"at": {"fn": "seconds", "args": ["0"]}, "text": "4", "color": "2"}
  },
  "list": {"from": "document.segments[*]", "url": "url", "count": "count", "maxSegments": 40}
}
```

引擎负责下载清单、按 `list.from` 取窗口、把每个窗口的文档绑定成 `document`、跑 `steps`、再用 `output` 映射。`steps` 因此可以是空的：窗口已经由引擎取到，规则不必再写一次 `request`。窗口相对 `manifestUrl` 解析，走的是绝对地址分支，不继承源签名——文件服务器不属于目录的域，把目录的签名发给它是错的。

选中器新增一种形态：整条选择器就是一个数字，读当前 item 的下标。这是为紧凑数组准备的，`"4"` 就是第六条。

数值换算靠新函数 `seconds`，把毫秒变成秒，保留小数（312.5s 不会变成 313s）。`at` 是时长，所以映射里给的值按秒解释，小数原样保留。

播放页的取值顺序从「先站外、再规则」改成三段：源自己的时间轴 → 站外匹配 → 规则的 `comments`。实现是可选接口 `source.Manifest`，播放页做类型断言：

```go
type Manifest interface {
    Manifest(ctx context.Context, episode Episode, limit int) ([]Danmaku, error)
}
```

它是独立接口而不是塞进 `source.Source`，和 `Commenter` 同一个理由：不该逼所有源都实现。只有 `Commenter` 的源行为完全不变。

## 后果

kanju 那一集的弹幕从 0 条变成 205 条带时间点的真弹幕，播放页不再和别人的投稿对齐。代价是规则多了一对块和一套窗口语义，`docs/rule-dsl.md` 相应多了一节；`rule.Block` 多了 `list`，`forEachBlock` 要给新块补默认值，漏掉就会让 `paginate` 的默认值不生效（`comments` 和 `updates` 原本就被漏掉了，这次一并补上）。

站外匹配仍然保留，红果这类没有时间轴弹幕的源照样走它；用户的偏好没有变，只是不再让它抢在源自己的数据前面。`danmaku` 包与 `commentWhy` 的闭集都不用改，因为不存在「源有弹幕但没用上」这个状态。

一个窗口下载失败只会少掉那几秒的弹幕，不会让整集变成空的——已经拿到的部分比一个坏文件重要得多。`maxSegments` 是兜底：清单被改坏、窗口数变得离谱时，播放页不会因此拉几十个文件。

判断要不要新增块的标准是「上游是否按时间轴托管弹幕」。若以后遇到分页式的弹幕接口（带 cursor 逐页取），`list` 的 `from` 表达能力不够，那时要加的是 `paginate` 的对应形态，不是往这里塞特例。

同一集切换线路或重进播放页会重拉清单。清单带 5 分钟缓存的话由源自己的 HTTP 缓存决定，规则与引擎都不做额外缓存；若实测发现拖动进度条时请求数不可接受，再在 `internal/danmaku` 或引擎层加入口缓存，位置留给那时候的证据。
