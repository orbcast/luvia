#!/usr/bin/env python3
"""检查仓库里那些"agent 最容易悄悄破坏"的约定。

这些规则单靠 AGENTS.md 的散文字句拦不住，所以做成可重复执行的检查，
由 make check、pre-commit 钩子和 CI 一起调用。每个检查只读仓库，
不联网、不修改文件；发现问题就列出位置并以状态码 1 退出。

检查项：
  1. 版本库不含生成物、日志、临时目录和编辑器残留；
  2. Markdown 的相对链接指向真实文件；
  3. docs/README.md 与 docs/adr/README.md 索引覆盖目录内文档；
  4. docs/adr/ 的文件名符合 NNNN-kebab-case.md；
  5. AGENTS.md 不超过体量预算（根 9000 字符，应用/模块 6000 字符）；
  6. apps/ 下非测试 Go 文件的字符串字面量不含中日韩文字（文案走 catalogs）；
  7. apps/ 下集中 Token 文件之外不出现十六进制颜色（配色走 DESIGN 语义 Token）；
  8. 各语言 catalogs 的键与源语言一致（MyGo 对缺失译文只告警，这里要拦住）。
"""

from __future__ import annotations

import json
import os
import re
import subprocess
import sys

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))

AGENTS_BUDGET_ROOT = 9000
AGENTS_BUDGET_INNER = 6000

# 生成物、缓存、临时文件与编辑器残留。忽略规则不授权存放它们，这里做最后一道拦截。
JUNK = [
    (r"^apps/[^/]+/(build|\.mygo)/", "构建产物应留在被忽略的目录，不进版本库"),
    (r"^\.pi/", "本地 Agent 状态不进版本库"),
    (r"(^|/)\.DS_Store$", "系统杂物"),
    (r"(^|/)__pycache__/", "Python 字节码缓存"),
    (r"\.pyc$", "Python 字节码缓存"),
    (r"(^|/)(tmp|\.tmp)/", "临时目录必须放在 OS 临时区域"),
    (r"\.log$", "日志"),
    (r"\.(test|prof|out)$", "测试二进制、性能与覆盖率输出"),
    (r"\.(bak|orig|rej|swp|swo)$", "编辑器与冲突残留"),
]

# 集中定义语义 Token 的文件，颜色只允许出现在这里（当前还没有这样的文件）。
TOKEN_FILES = {"theme.go", "tokens.go", "design.go", "design_tokens.go"}

HEX_COLOR = re.compile(r"#[0-9a-fA-F]{3,8}\b")
MD_LINK = re.compile(r"\[[^\]]*\]\(([^)#\s]+)(#[^)]*)?\)")
ADR_NAME = re.compile(r"^\d{4}-[a-z0-9]+(-[a-z0-9]+)*\.md$")


def git(*args: str) -> list[str]:
    out = subprocess.run(
        ["git", *args], cwd=ROOT, check=True, capture_output=True, text=True
    )
    return [line for line in out.stdout.splitlines() if line]


def tracked() -> list[str]:
    return git("ls-files")


def failures() -> list[str]:
    problems: list[str] = []
    files = tracked()

    for path in files:
        for pattern, why in JUNK:
            if re.search(pattern, path):
                problems.append(f"{path}: {why}")
                break

    for path in files:
        if path.endswith(".md") and not path.startswith(".agents/skills/"):
            # `.agents/skills/` 是随 CLI 分发的技能，其中的示例路径不指向本仓库。
            problems.extend(check_links(path))

    problems.extend(check_index("docs/README.md", "docs", skip=("docs/adr/",)))
    problems.extend(check_index("docs/adr/README.md", "docs/adr"))
    problems.extend(check_adr_names(files))
    problems.extend(check_agents_budget(files))
    problems.extend(check_go_sources(files))
    problems.extend(check_catalogs(files))
    return problems


def check_links(rel: str) -> list[str]:
    problems = []
    base = os.path.dirname(rel)
    try:
        text = open(os.path.join(ROOT, rel), encoding="utf-8").read()
    except OSError as err:
        return [f"{rel}: 无法读取: {err}"]
    for match in MD_LINK.finditer(text):
        target = match.group(1)
        if target.startswith(("http://", "https://", "mailto:", "tel:")):
            continue
        resolved = os.path.normpath(os.path.join(base, target))
        if not os.path.exists(os.path.join(ROOT, resolved)):
            problems.append(f"{rel}: 链接目标不存在: {target}")
    return problems


def check_index(index_rel: str, scope: str, skip: tuple[str, ...] = ()) -> list[str]:
    """目录内的正式文档都必须在索引里有链接，避免文档悄悄失联。"""
    index_path = os.path.join(ROOT, index_rel)
    if not os.path.exists(index_path):
        return [f"{index_rel}: 缺少索引文件"]
    text = open(index_path, encoding="utf-8").read()
    problems = []
    for dirpath, dirnames, filenames in os.walk(os.path.join(ROOT, scope)):
        dirnames[:] = [d for d in dirnames if d not in {".git", "build", "__pycache__"}]
        for name in sorted(filenames):
            if not name.endswith(".md"):
                continue
            rel = os.path.relpath(os.path.join(dirpath, name), ROOT).replace(os.sep, "/")
            if rel == index_rel or rel.startswith(skip) or rel == "docs/adr/README.md":
                continue
            if name not in text:
                problems.append(f"{index_rel}: 索引漏掉 {rel}")
    return problems


def check_adr_names(files: list[str]) -> list[str]:
    problems = []
    seen: dict[str, str] = {}
    for path in files:
        if not path.startswith("docs/adr/") or not path.endswith(".md"):
            continue
        name = os.path.basename(path)
        if name == "README.md":
            continue
        if not ADR_NAME.match(name):
            problems.append(f"{path}: ADR 文件名应为 NNNN-kebab-case.md")
            continue
        number = name[:4]
        if number in seen:
            problems.append(f"{path}: 编号与 {seen[number]} 重复")
        seen[number] = path
    return problems


def check_agents_budget(files: list[str]) -> list[str]:
    problems = []
    for path in files:
        if os.path.basename(path) != "AGENTS.md":
            continue
        budget = AGENTS_BUDGET_ROOT if path == "AGENTS.md" else AGENTS_BUDGET_INNER
        size = len(open(os.path.join(ROOT, path), encoding="utf-8").read())
        if size > budget:
            problems.append(
                f"{path}: {size} 字符，超过 {budget} 字符预算；先合并或删除失效规则"
            )
    return problems


def string_literals(source: str):
    """产出 (起始行号, 字面量内容)，跳过注释、rune 与非解释型字符串。"""
    i, line, n = 0, 1, len(source)
    while i < n:
        ch = source[i]
        if ch == "\n":
            line += 1
            i += 1
        elif source.startswith("//", i):
            while i < n and source[i] != "\n":
                i += 1
        elif source.startswith("/*", i):
            end = source.find("*/", i + 2)
            end = n if end < 0 else end + 2
            line += source.count("\n", i, end)
            i = end
        elif ch == "`":
            end = source.find("`", i + 1)
            end = n if end < 0 else end
            yield line, source[i + 1 : end]
            line += source.count("\n", i, end)
            i = end + 1
        elif ch == '"':
            start, j, buf = line, i + 1, []
            while j < n:
                if source[j] == "\\":
                    buf.append(source[j : j + 2])
                    j += 2
                    continue
                if source[j] == '"':
                    break
                if source[j] == "\n":
                    break
                buf.append(source[j])
                j += 1
            yield start, "".join(buf)
            i = j + 1
        else:
            i += 1


CJK = re.compile(r"[\u3000-\u303f\u3400-\u9fff\uf900-\ufaff\uff01-\uff60]")


def check_go_sources(files: list[str]) -> list[str]:
    problems = []
    for path in files:
        if not path.endswith(".go") or not path.startswith("apps/"):
            continue
        if path.endswith("_test.go"):
            continue
        source = open(os.path.join(ROOT, path), encoding="utf-8").read()
        for line, literal in string_literals(source):
            if CJK.search(literal):
                problems.append(
                    f"{path}:{line}: 字符串里有中文文字，用户可见文案应走 locales 的 catalog"
                )
            if os.path.basename(path) not in TOKEN_FILES:
                for color in HEX_COLOR.findall(literal):
                    problems.append(
                        f"{path}:{line}: 十六进制颜色 {color}，配色应由 DESIGN.md 的语义 Token 提供"
                    )
    return problems


def catalog_dirs(files: list[str]) -> list[str]:
    return sorted(
        {
            os.path.dirname(path)
            for path in files
            if path.endswith(".json") and "/locales/" in path
        }
    )


def check_catalogs(files: list[str]) -> list[str]:
    """每份译文都要覆盖源语言的每个键，不靠 fallback 蒙过去。"""
    problems = []
    for directory in catalog_dirs(files):
        config = os.path.join(os.path.dirname(directory), "mygo.json")
        source = "en"
        if os.path.exists(os.path.join(ROOT, config)):
            data = json.load(open(os.path.join(ROOT, config), encoding="utf-8"))
            source = data.get("i18n", {}).get("source", source)
        base_path = os.path.join(ROOT, directory, source + ".json")
        if not os.path.exists(base_path):
            problems.append(f"{directory}: 缺少源语言 {source}.json")
            continue
        base = json.load(open(base_path, encoding="utf-8"))
        keys = {key for key in base if not key.startswith("@")}
        for name in sorted(os.listdir(os.path.join(ROOT, directory))):
            if not name.endswith(".json") or name == source + ".json":
                continue
            path = f"{directory}/{name}"
            try:
                other = json.load(open(os.path.join(ROOT, path), encoding="utf-8"))
            except json.JSONDecodeError as err:
                problems.append(f"{path}: 不是有效 JSON: {err}")
                continue
            translated = {key for key in other if not key.startswith("@")}
            for key in sorted(keys - translated):
                problems.append(f"{path}: 缺少译文 {key}")
            for key in sorted(translated - keys):
                problems.append(f"{path}: 源语言没有 {key}，多余的键会一直无人使用")
    return problems


def main() -> int:
    problems = failures()
    if not problems:
        print(f"check-rules: 全部通过（{len(tracked())} 个文件）")
        return 0
    for problem in problems:
        print(f"check-rules: {problem}", file=sys.stderr)
    print(f"check-rules: {len(problems)} 个问题", file=sys.stderr)
    return 1


if __name__ == "__main__":
    sys.exit(main())
