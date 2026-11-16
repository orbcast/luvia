#!/usr/bin/env python3
"""按语句统计覆盖率，生成代码不计入分母。

`go test -cover` 的 total 会把 `*_gen.go` 与 catalogs 目录一起算进去，那些
代码没有也不该有手写测试，混进来只会让门槛失真。这里直接解析
coverprofile，只统计手写代码。

用法：python3 -B scripts/check-coverage.py --profile cover.out --min 80
"""

from __future__ import annotations

import argparse
import re
import sys

GENERATED = re.compile(r"(_gen\.go$|/locales/)")


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--profile", required=True)
    parser.add_argument("--min", type=float, default=80.0)
    args = parser.parse_args()

    per_file: dict[str, list[int]] = {}
    total = covered = 0
    with open(args.profile, encoding="utf-8") as profile:
        header = profile.readline().strip()
        if not header.startswith("mode:"):
            print(f"check-coverage: {args.profile} 不是 coverprofile", file=sys.stderr)
            return 1
        for line in profile:
            line = line.strip()
            if not line:
                continue
            location, statements, count = line.rsplit(" ", 2)
            path = location.split(":")[0]
            if GENERATED.search(path):
                continue
            size, hits = int(statements), int(count)
            total += size
            per_file.setdefault(path, [0, 0])
            per_file[path][0] += size
            if hits > 0:
                covered += size
                per_file[path][1] += size

    if not total:
        print("check-coverage: 没有统计到任何语句", file=sys.stderr)
        return 1

    for path in sorted(per_file):
        size, hits = per_file[path]
        print(f"  {hits / size * 100:5.1f}%  {path}")
    percent = covered / total * 100
    verdict = "达标" if percent + 1e-9 >= args.min else "低于门槛"
    print(
        f"check-coverage: {percent:.1f}% of {total} statements"
        f"（最低 {args.min:.0f}%，生成代码不计）：{verdict}"
    )
    return 0 if percent + 1e-9 >= args.min else 1


if __name__ == "__main__":
    sys.exit(main())
