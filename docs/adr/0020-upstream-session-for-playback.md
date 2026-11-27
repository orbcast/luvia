# 0020 播放线路需要的上游会话由引擎自行申领

状态：已采纳
日期：2026-11-27

## 背景

kanju 的部分内容（综艺新集、独家片）只提供 `resolve_mode: "parse"` 的线路，兑换票据时上游回

```
401 {"code":"playback_user_session_required","state":"login_required","message":"该内容需要登录后播放"}
```

浏览器能播，不是因为登录了账号，而是因为它在首次打开站点时自动领了一份游客身份：`POST /v1/users/anonymous`（body 只有 `{anonymous_id}`，一个 `web_<uuid4>`）返回 201 与

```
Set-Cookie: ai_movie_session=ums_...; Path=/; HttpOnly; SameSite=Lax; Max-Age=31535999; Secure
```

有效期一年，`GET /v1/users/me` 回 `authenticated: false`——这就是匿名会话，和任何个人账号无关。同一张票据，不带来会话兑换是 401，带上就 201 并给出真实地址（实测 74 MB 的 mp4，`206` + `ftypisom`）。

所以拒绝的不是「没有人类账号」，而是「没有身份」。规则有签名没会话，就永远兑换不了这类线路。

## 决定

规则新增一个可选的顶层 `session` 块，描述**怎么向上游申领一份匿名会话**；引擎负责发请求、记下 `Set-Cookie`、在之后的请求里带上，并把 cookie 存进用户数据目录，下次启动复用。

```json
"session": {
  "request": {
    "method": "POST",
    "path": "/v1/users/anonymous",
    "body": {"anonymous_id": {"fn": "anonId"}}
  },
  "cookie": "ai_movie_session",
  "ttl": "expires_at"
}
```

- `request` 就是普通的请求描述（与步骤、线路兑换同一套字段），照常签名。`{fn: "anonId"}` 是新增的零参函数：首次调用生成 `web_<uuid4>`，同一个引擎实例内稳定，并发调用拿到同一个值。
- `cookie` 是要保留的 cookie 名；上游先发过期 set-cookie 再发新的，只有最后那个非空值进缓存。
- `ttl` 可选，是这个响应里表示过期时刻的字段（RFC 3339）。省略时按 cookie 自身的 `Max-Age`/`Expires` 推算。

发动机在第一次需要会话的请求前申领一次，之后复用；响应 401 且规则声明了 `session` 时，丢弃缓存、重新申领、重试一次请求（只重试一次，避免被上游限速时打转）。缓存在用户数据目录的 `sessions.json`（权限 0600，与 `secrets.json` 同级），键是源 ID。

同时补两件配套的事：

- 请求描述新增 `cookie: false`，让某个请求不带会话（上游的 cookie 是 `Path=/`，会盖到所有 `/v1/`，个别接口可能不该带）。
- 线路兑换出来的地址是直连 mp4 时，播放器已经用本地缓存代理拉流；kanju 的 CDN 有 Referer ACL，**带 `Referer: https://kanju1.com/` 反而 403，不带才 206**。这条属于规则作者的事，写进 `apps/luvia-rules/kanju/README.md`，引擎不做特殊处理。

## 后果

租来的身份让「需要登录」的内容在完全匿名下可播，规则因此不需要用户提供账号、cookie 或扫码，和 [ADR 0019](0019-access-keys-in-the-app.md) 的「密钥不落盘」也不冲突：`anonymous_id` 是随机 UUID，会话是上游发给这个匿名身份的，五个人跑出来是五个不同的「林间来客xxxx」。

代价有三条。一是规则从此会写用户目录里的 `sessions.json`，引擎从「只读上游」变成了「有本地持久状态」，多机份文件、权限、清理都要按 `secrets.json` 的标准维护。二是会话的边界由上游决定：`Max-Age` 一年不等于一年都有效，上游随时能让 token 失效，失效策略只能靠「401 就重建、只重建一次」来兜，重建也失败的报错必须指向「上游拒绝发放游客身份」而不是「需要登录」，否则用户会去设置里找根本不存在的登录入口。三是这条能力天然适合被滥用：它把「匿名可播」固化成产品的默认路径，上游一旦收紧游客配额或加人机校验，规则会在所有用户那里同时坏掉，而不是只坏一个人。

重新评估的触发条件：上游给 `users/anonymous` 加人机校验或按 IP 限流；某天需要「同一个游客身份跨设备连续」；或者要接第二家需要会话的源，那时把 `sessions.json` 抽成共享模块，而不是在规则里复制第二份。
