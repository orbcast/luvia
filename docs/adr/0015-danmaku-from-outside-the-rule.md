# 0015 弹幕按标题在站外匹配，规则只提供剧集

- 状态：已采纳
- 日期：2026-11-21

## 背景

播放页的弹幕层只有一个来源：`source.Commenter`，也就是源规则的 `comments` 块。实际接上红果后这层几乎是空的，原因不在实现而在数据——红果的 `comments` 块取的是 `episodeSocialInfo.reviews`，是剧评，没有时间点。`internal/player/page.go` 的 `timedComments` 因此恒为假，弹幕走 `spreadMoment` 把几条剧评均匀铺在整集时长上，同时右栏评论页签里也就那几条。官方弹幕接口在 `api5-normal-sinfonlineb.fqnovel.com`，要复刻整套签名与设备指纹（见 [hongguo README](../../apps/luvia-rules/hongguo/README.md)），不在规则能表达的范围内。

同时用户报了两个具体的界面问题：弹幕按钮按下去看不出变化，评论页签为空时只说「暂无内容」。前者的原因是 `pillButton` 的 active 态只有 16% 白的底，压在明亮的画面上与未选中没有区别；后者是 `p.commentsNote` 写了四处却零处读取，于是「这个源没有弹幕」这件事从未告诉过观众。这两点让「没有弹幕」和「弹幕坏了」在界面上完全无法区分。

## 决定

新增 `apps/desktop/internal/danmaku`，把弹幕来源从源规则里独立出来，成为一个按标题去站外找的接口：

```go
type Source interface {
    Lookup(ctx context.Context, title string, episode int) (Result, error)
}
```

`Result` 带 `Items []source.Danmaku` 与 `Reason`，`Reason` 是闭集而不是消息：`ReasonReady`、`ReasonNoMatch`、`ReasonNoEpisode`、`ReasonEmpty`、`ReasonFailed`。包只知道发现了什么，用词归播放页。

实现是 B 站：搜索标题 → 排序候选 → 前几个打开分 P 列表 → 按分 P 取弹幕 → 转成 `source.Danmaku`。弹幕走 `x/v2/dm/web/seg.so`，按 `segment_index` 分段取回归一化的 protobuf（字段 2 时刻毫秒、3 模式、4 字号、5 颜色、7 正文），只留 mode 1/4/5（滚动、底部、顶部），其余播放器画不出来；响应是裸 deflate 且 `Content-Encoding: deflate` 不按 zlib 头包装，`decompress` 按 zlib 头魔数判断再退回 `flate.NewReader`。段数由分 P 时长推算（每段约覆盖 6 分钟，上限 24 段），读到空段即止。

选片靠三次搜索合并：剧名、`剧名 第N集`、`剧名（1-100）`。站的性质是搜索引擎，剧名单搜会被 reaction、解析、壁纸挤满，站自己的合辑沉在后面；`（1-100）` 这种范围写法是站方目录给整季投稿的命名。评分按标题命中强度、是否“剧名+集数范围”、分 P 数（封顶 200）、时长、播放量、投稿年龄综合；`（1-100）` 这类范围命中再额外加权，把站方自己的合辑顶到前面——有声剧、reaction 合集的分 P 数比正片还多，不能只靠数量决定。前几个候选（`maxProbes=5`）先用 `pagelist` 探分 P，再按原始排名逐个尝试（`maxTries=5`），遇到没有这一集、或这一集没有弹幕就换下一个，不再由第一个命中单独拍板。

播放页的取值顺序是「先站外、再规则」：站外拿到 `ReasonReady` 就用它；站外说 `ReasonNoEpisode`/`ReasonEmpty`/失败时，规则的剧评仍然有机会填进层里；两边都没有时，把站外的 `Reason` 交给面板。`commentWhy` 取代了原来的 `commentsNote` 死字段，面板按它说「没找到这部剧的弹幕源」「找到了这部剧，但没有这一集」「这一集还没有弹幕」「弹幕源没连上」「这个源没有弹幕」。

`pillButton` 的 active 态从 16% 白底改成实心反白（`Background(mediaInk)` + 深色图标），hover 态才用半透明白。开关打开这件事必须在明亮画面上看得出来。

## 后果

弹幕不再依赖规则是否有 `comments` 块，也不需要改 `apps/luvia-rules` 下任何规则；换一个没有评论的源，弹幕层照样有东西。代价是标题匹配本身不可靠：站上的投稿命名随意（`【1-7季】全球死考（最全版）`、`全球死考：刚下刑场你让我去考试-3`），`titleMatch` 用最长公共子串加最小共享四字来判定，集数靠 part 标签里的数字或范围（`1-4`）匹配，标签全无数字时退回按位置取。同名不同剧、或者投稿把多季塞进一个 part 的情况会匹配错，此时观众看到的是别人的弹幕——这是本决策接受的代价，因为它比「什么都看不到」更接近观众想要的东西，而且右栏评论页签与弹幕层同源，错配一眼可见。

`danmaku` 包只依赖标准库与 `internal/source`，不 import `player`，播放页通过 `player.Deps.Danmaku` 注入，`screens.Deps.Danmaku` 透传，`pages.go` 构造 `danmaku.NewBilibili(nil)`。nil 表示不做站外查找，行为回到旧路径。

站外请求是普通 HTTP，带桌面 Chrome UA 与 `Referer: https://www.bilibili.com`；端点、UA 与 Referer 是公开可查的技术事实，写在这里是因为它们是这个包的接口地址而不是任何源的凭证。站上若改端点或加签名，改的是 `internal/danmaku/bilibili.go` 的常量，不影响规则与播放页。`scripts/check-rules.py` 的 `UPSTREAM_FILES` 把 `bilibili.go` 排除在中文字面量检查之外：那里的中文是发往上游的查询词与匹配标题的正则，不是界面文案。

缓存按 cid 记住已拉到的弹幕，上限 24 集，超出按入队顺序淘汰。观众来回拖动进度条会反复请求同一集，没有缓存的话每次都是 200KB 级别的重复下载。

站外弹幕与规则剧评在同一个层里，因此「弹幕开关」同时控制两者；这不是新行为，只是现在这一层终于有东西可关了。观众若只想要站上的弹幕而不想看到剧评，需要另做一个来源选择，目前没有这个需求。
