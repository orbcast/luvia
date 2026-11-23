# 0018 弹幕层自己走播放头，不等播放器的报告

- 状态：已采纳
- 日期：2026-11-23

## 背景

观众反馈弹幕像幻灯片一样一顿一顿地飘过。第一次排查发现整页每 200 毫秒才重绘一次（`apps/desktop/internal/player/wait.go` 的 `waitTick`），5fps 下弹幕位置只能一秒跳五步；改成 16 毫秒一帧（`playTick`）之后画面和弹幕层统一到屏幕的节拍。但帧率只是问题的一半。

弹幕位置每帧都从播放头算：`flyingComments(p.position(), ...)`。而 `ui.VideoPlayer.Position()` 是系统播放器给的一个采样值，报告频率远低于每秒 60 次。帧率再高，只要位置是那几次报告里的常量，弹幕就还是只在报告来的那几帧动——16fps 的插帧只是把同样的阶梯踩得更密，卡顿依然存在。

## 决定

`apps/desktop/internal/player/layerclock.go` 新增 `layerClock`：记下最后一次报告的播放位置与单调时钟读数，两帧之间按倍率自行推进（`at + (now - sampled) * rate`），弹幕层用这个位置布局，不再直接用 `p.position()`。这是字幕与弹幕渲染的通行做法——weizhenye/Danmaku 的 `raf` 主循环把 `timestamp` 每帧传进引擎现算位置；B 站 DanmakuFlameMaster 默认 `updateMethod = 0`，由 `Choreographer` 的 vsync 回调驱动重绘；Flutter 的 `canvas_danmaku` 用 `createTicker`。共同点是位置跟着屏幕走，不跟着播放器的采样点走。

时钟只在两种情况下重新锚定：报告落点超出 `sampleTolerance`（400ms）说明不是走路能到的位置，即拖动进度条或没预告的卡顿；报告倍率变了，即切换播放速度。慢采样（报告值落后于推算值）不算跳变——那正是要修的情形，若每次都settle回报告值，弹幕会在报告到达的那一帧往回退一步。

`Page.now` 是注入的时钟，测试用 `SetClock` 换成手动推进的假时钟，这样「每帧位移是否均匀」可以被断言而不是靠肉眼看。

## 后果

弹幕逐帧匀速平移，2 倍速下按两倍速走，暂停时停在原处不再滑过画面。代价是时钟是推算值，播放器若长时间不报位置而速度悄悄变了，推算会偏，直到下一次报告把它纠正回来——偏的是几百毫秒内的弹幕相位，不是时间轴本身的错位，且下一次报告就修正。另一个代价是 `flyingComments` 的入参语义变成「层认为的播放头」，不再是播放器的原值；`entry` 字段被带进 `flying`，让视图从入场时刻和当前播放头重算 slide，帧跳过一格也不会让位移抖动。

`internal/player/layerclock_test.go` 覆盖五种情形：慢采样之间的插值、迟到报告不倒退、拖动后重新锚定、倍速下按倍率走、报告停滞时层停住。`tests/player_danmaku_flow_test.go` 用假时钟逐帧断言位移恒定且为正，删掉插值后第一条断言即失败。两组测试都不测真实播放器的报告节奏，那属于平台行为，靠实机观察与 `playTick` 的帧率保证。
