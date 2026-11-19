# 0011 超分与杜比止步于系统播放层

- 状态：已采纳
- 日期：2026-11-19

## 背景

超分辨率与杜比视界都需要在解码后、呈现前介入：超分要拿一帧 YUV/BGRA 纹理再放大，杜比要拿到 10-bit PQ 码值与逐帧 RPU 再做 reshape，两者都要求应用自己持有解码帧、时钟和呈现目标。调研的四家开源播放器（mpv、Kodi、VLC、MPC Video Renderer）无一例外都建立在这个前提上：mpv 用 libplacebo 的 `pl_shader_dovi_reshape` 与 `sh_dovi_compose_nlq`，Kodi 用系统的 Dolby Vision 渲染效果 MFT，MPCVR 自己封装 `ID3D11VideoProcessor` 调 NVIDIA 私有扩展，avox 直接自建 Vulkan 解码与呈现链。

桌面版 `apps/desktop` 走 MyGo 的 `ui.VideoPlayer`，Windows 侧由 Media Foundation 的 frame-server 模式硬解：`ui/video/mf_windows.go:436 Frame(t time.Time)` 忽略时间参数，直接问引擎有没有新帧，时钟与解码都在 MF 内部；每帧经 `TransferVideoFrame` 转成 BGRA 写入三张带 NT handle 与 keyed mutex 的共享纹理；`Picture.Surface` 是渲染器消费的最终结果，没有替换通道。也就是说，10-bit、色度与 RPU 在交给应用之前就已经丢失。

## 决定

不为超分与杜比重写播放层，也不引入 libmpv。Windows 上保留系统播放链，把可验证的能力边界写清楚，不通过界面文案暗示做了没做的事。

为确认这条边界不是猜的，2026-11-19 在 RTX 4060 Laptop（驱动 610.88、Windows 11 26200）上做过一轮独立探针：自建 D3D11 设备用 `OpenSharedResource1` 打开 MyGo 引擎导出的共享 BGRA 纹理，建 `ID3D11VideoProcessor` 并 `VideoProcessorBlt` 到自有纹理再读回——整条链成功，像素确实跨设备送达（边缘能量 7.4759），`CheckVideoProcessorFormat` 对 BGRA 与 NV12 都返回输入输出双向支持。但 NVIDIA 私有扩展 `VideoProcessorSetStreamExtension(kNvidiaPPEInterfaceGUID, {version=1, method=2, enable=1})` 虽然返回 S_OK，按位域读回状态是 `capable=1 valid=1 enabled=0 inUse=0 level=0`：驱动认得接口、GPU 声明支持，却不放行。伪造 GUID 的同一调用同样返回 S_OK，所以 S_OK 本身不能当作成功判据，只有位域读回可信。补齐 MPCVR 的全套 stream state、换 NV12 输入、连喂多帧、把进程改名成 `vlc.exe`/`mpc-hc64.exe`/`chrome.exe`，结果完全一致。

## 后果

播放链保持系统原生：AirPlay、画中画、HDR 自动切换、硬解与电量管理都继续由 Media Foundation / AVFoundation 负责，不必为每个平台自编解码与呈现层。

代价是 Windows 上拿不到杜比视界的正确呈现，也拿不到自研超分。杜比方面，桌面 Windows 没有 Dolby Vision 输出 API（Kodi 的 `SetDolbyVisionOutput` 走 `HdmiDisplayInformation`，整条路径被 `#if defined(TARGET_WINDOWS_STORE)` 包住且先判 `GetWindowsDeviceFamily() != Xbox` 直接返回 false），能追求的上限是"DV P5/P8 正确解成 PQ BT.2020 的 HDR10 输出"，而这一步在 MF 交出 BGRA 之后已经无法完成。

超分方面，驱动级 RTX VSR 按应用放行，NVIDIA 的 DRM 数据库里没有 Luvia 的条目，第三方进程即使调通接口也拿不到 `enabled=1`；官方给第三方集成的是 NVIDIA RTX Video SDK（需随包分发 `nvngx_vsr.dll`，受 RTX SDK 许可约束），与这条私有 GUID 不是一回事。

重新评估的触发条件：一，NVIDIA RTX Video SDK 的分发与许可条款被接受，那时超分可以只替换渲染端而不动解码；二，MyGo 提供带时间戳的解码帧通道，那时两条路都重新变得可行；三，产品要求改成"必须自带播放内核"，那就是引入 libmpv 的决策，与本文无关。

探针只在 NVIDIA 上做过，Intel 的 `SetSuperResIntel` 与 AMD 的路径未验证；结论限定为"这条路在本项目当前播放层上不成立"，不是"NVIDIA 驱动永远不放行"。
