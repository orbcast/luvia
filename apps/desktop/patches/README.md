# 本地补丁

本目录存放需要打在依赖源码上的补丁，因为 Go 模块缓存在 `~/go/pkg/mod` 下只读，
且 `go.mod` 的 `replace` 一旦指向绝对路径就会把本机目录结构写进仓库、
在别人的机器上也失效。补丁本身作为文本存在仓库里，由脚本应用到模块缓存。

## mygo-gst-offsets.patch

`github.com/egoist/mygo@v0.10.0` 的 Linux 播放后端 `ui/video/gst_linux.go`
把两个手写的 ABI 偏移量写错了：

| 常量 | 上游值 | 正确值 | 后果 |
| --- | --- | --- | --- |
| `gstBufferPTSOffset` | 72 | 24 | `pull()` 判重读不到真 PTS，新帧被当重复丢弃 |
| `gstMessageTypeOffset` | 64 | 16 | `handle()` 匹配不到 `ASYNC_DONE` / `DURATION_CHANGED` |

`GstMiniObject` 在 amd64 上是 16 字节（`GType` 8 + `refcount` 4 + `lockstate` 4），
因此 `GstBuffer.pts` 在 +24，`GstMessage.type` 在 +16。读错的表现是
`VideoPlayer.Ready()` 恒 false、`Position()` 恒 0：画面能出第一帧但进度不动。

只影响 Linux；macOS 走 AVFoundation、Windows 走 Media Foundation，
不使用手写结构体偏移。

## 应用方式

```sh
MODDIR=$(go list -m -f '{{.Dir}}' github.com/egoist/mygo)
chmod -R u+w "$MODDIR/ui/video"
patch -d "$MODDIR" -p1 < apps/desktop/patches/mygo-gst-offsets.patch
```

升级 mygo 后需要重新确认补丁是否仍然适用；上游修好后本文件即可删除。
