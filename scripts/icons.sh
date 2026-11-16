#!/bin/sh
# 从 assets/icons/ 的共享母版生成各应用需要的图标文件。
# 母版是矢量 SVG + 无损 WebP，但 actool 与 MyGo 只吃 PNG/局部格式，所以这里做一次同步。
set -eu

cd "$(dirname "$0")/.."

MASTERS=assets/icons
APP=apps/desktop

png() {
	src=$1
	dst=$2

	if [ ! -f "$src" ]; then
		echo "icons: 缺少母版 $src" >&2
		exit 1
	fi

	mkdir -p "$(dirname "$dst")"

	if command -v dwebp >/dev/null 2>&1; then
		dwebp -quiet "$src" -o "$dst"
	elif command -v python3 >/dev/null 2>&1 && python3 -c 'import PIL' 2>/dev/null; then
		python3 -c 'import sys; from PIL import Image; Image.open(sys.argv[1]).convert("RGBA").save(sys.argv[2])' "$src" "$dst"
	else
		echo "icons: 需要 dwebp（brew install webp）或带 Pillow 的 python3" >&2
		exit 1
	fi

	echo "icons: $src -> $dst"
}

# macOS：矢量标志交给 Icon Composer 文档，方图底色和圆角、玻璃高光都由系统图标栈负责
cp "$MASTERS/mark.svg" "$APP/AppIcon.icon/Assets/mark.svg"
echo "icons: $MASTERS/mark.svg -> $APP/AppIcon.icon/Assets/mark.svg"

# Windows / Linux / 开发版：黑色超椭圆底图（这些平台没有系统图标遮罩）
png "$MASTERS/app-icon-dark.webp" "$APP/resources/icon.png"
