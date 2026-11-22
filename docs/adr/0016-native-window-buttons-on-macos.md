# 0016 macOS 使用系统原生红黄绿按钮

- 状态：已采纳
- 日期：2026-11-22

## 背景

桌面框架最初在三个平台统一隐藏系统窗口按钮（`mygo.WindowOptions.HideWindowButtons`），由 `internal/appshell` 在标题栏一角自绘一颗玻璃胶囊，内含最小化、最大化、关闭三枚按钮（见 [DESIGN.md](../../DESIGN.md) 与 [desktop README](../../apps/desktop/README.md)）。这个做法在 Windows 与 Linux 上成立：这些桌面的窗口按钮本来就由应用按主题自己画，样式各异。

在 macOS 上它不成立。系统红黄绿按钮不只是三枚控件：悬停显示符号、双击缩放、绿灯的窗口排列菜单、进入全屏时的显隐、最小化到 Dock 的动画、辅助功能与键盘导航，全部由 AppKit 提供。自绘版本只能调用 `Minimize`、`ToggleMaximize`、`Close`，这些行为都要应用自己重做一遍，而且位置、间距、配色和系统版本之间的差异永远对不齐；用户看到的是「不像 Mac 的窗口」。MyGo 的 `TitleBarHidden` 本来就能保留系统按钮并让页面铺满窗口，`ui.Context.TitleBar` 会报告它们占用的范围（实测 1100 × 720 窗口下 `{Height:32 Left:69 Bottom:23}`），应用只需要避开这块区域。

## 决定

按平台分工：`appshell.NativeWindowButtons(goos)` 在 macOS 返回真，窗口以 `TitleBarHidden` 创建且**不**隐藏系统按钮，`appshell` 也不接收按钮布局，因此不画胶囊；Windows 与 Linux 保持原做法，隐藏系统按钮并按桌面布局自绘。

顶线 `design.ChromeTop` 相应改成：系统标题栏按钮存在时取「其底边 + `ShellInset`（12）」，否则取 `WindowControlTop`（12）。左列导航、详情页返回按钮都从这条线开始，因此不会贴到红黄绿上；窗口拖动条仍从 `bar.Left` 起，红绿灯本身可点。

自绘胶囊的 Token 与 `internal/glass` 的 `Surface` 保留，只服务 Windows 与 Linux；`Shell.SetWindowControlsHidden` 在 macOS 上不再隐藏原生按钮，播放页期间红绿灯留在左上角，进入系统全屏时由 AppKit 隐藏。

## 后果

代价是三个平台的外观不再一致：macOS 的窗口按钮看不出来自应用主题，Windows 与 Linux 仍是自绘胶囊。这是有意的，系统原生控件在自己的平台上比统一皮肤更可靠。

`ChromeTop` 在系统按钮下方留的 12 个单位与面板距窗口左边缘的 12 相同，实测红绿灯底边距窗口顶部 23，导航列顶边因此落在 35，左上角读作等距内缩而非被按钮压住；先前的 6 个单位看起来仍偏紧。若以后改用 `TitleBarHiddenInset` 或给窗口设置 `TrafficLightPosition`，`TitleBar` 报告的底边会随之变化，`ChromeTop` 不需要改，但视觉间距需要在 macOS 上重新看一次。

macOS 上播放页无法隐藏红绿灯（MyGo 没有运行期切换 `HideWindowButtons` 的接口），沉浸感略低于 Windows 与 Linux。若这一点日后成为问题，需要在 MyGo 侧提供运行期隐藏，或让播放页进入系统全屏，而不是在本应用里再画一套控件。
