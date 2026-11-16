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
  8. 各语言 catalogs 的键与源语言一致（MyGo 对缺失译文只告警，这里要拦住）；
  9. 文档代码块里的 make 目标与 go tool mygo 子命令真实存在；
 10. apps/ 之间不互相导入，packages/ 不反向依赖 apps/；
 11. apps/ 下没有前端工程文件与 WebView 桥接（本项目用 Go 写原生界面）；
 12. 版本库没有私钥、访问令牌与 .env；
 13. 文档与脚本不写死本机路径；
 14. 单个文件不超过 1 MB（素材除外）；
 15. 应用入口 main.go 不超过 300 行；
 16. 文本文件统一 LF 且以换行结尾。
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

# 文档里出现的命令必须真的能用：make 目标从 Makefile 读，mygo 子命令在这里列全。
MAKE_TARGET = re.compile(r"^([a-zA-Z0-9][\w-]*):", re.M)
MAKE_CALL = re.compile(r"(?:^|[\s;&|(`])make\s+(?:-[a-zA-Z-]+\s+)*([a-zA-Z0-9][\w-]*)")
MYGO_CALL = re.compile(r"go\s+tool\s+mygo\s+([a-z][a-z0-9-]*)")
MYGO_COMMANDS = {
    "build",
    "dev",
    "doctor",
    "generate",
    "i18n",
    "init",
    "install-skills",
    "keygen",
    "migrate-ui",
    "native",
    "version",
    "vet",
}
COMMAND_BLOCK = re.compile(r"```(?:bash|sh|shell|console)\n(.*?)```", re.S)
LOCAL_SCRIPT = re.compile(r"((?:\.agents|scripts|assets)/[\w./-]+\.[a-z]{2,4})")

FRONTEND_FILES = {
    "package.json",
    "package-lock.json",
    "pnpm-lock.yaml",
    "yarn.lock",
    "bun.lockb",
    "tsconfig.json",
    "mygo.config.ts",
    "vite.config.ts",
}
FRONTEND_EXT = (".ts", ".tsx", ".js", ".jsx", ".html", ".css")
WEBVIEW_TOKENS = ("github.com/webview/", "mygo.Bind(", "mygo.NewEvent(", ".WebView(")

SECRETS = [
    (r"-----BEGIN [A-Z ]*PRIVATE KEY-----", "私钥内容"),
    (r"ghp_[A-Za-z0-9]{20,}", "GitHub 访问令牌"),
    (r"github_pat_[A-Za-z0-9_]{20,}", "GitHub 访问令牌"),
    (r"sk-[A-Za-z0-9]{20,}", "API 密钥"),
    (r"AKIA[0-9A-Z]{16}", "AWS 访问密钥"),
]
SECRET_FILES = (r"(^|/)\.env(\..+)?$", r"\.(pem|p12|keystore)$")

LOCAL_PATH = re.compile(r"(/Users/|/var/folders/|[A-Za-z]:\\Users\\|/home/[a-z0-9_-]+/)")
MAX_FILE_BYTES = 1024 * 1024
MAX_ENTRY_LINES = 300


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
    problems.extend(check_commands(files))
    problems.extend(check_dependency_direction(files))
    problems.extend(check_frontend_files(files))
    problems.extend(check_secrets(files))
    problems.extend(check_local_paths(files))
    problems.extend(check_sizes(files))
    problems.extend(check_text_hygiene(files))
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


def check_commands(files: list[str]) -> list[str]:
    """文档里的命令必须能跑：目标存在、子命令存在、脚本存在。"""
    problems = []
    makefile = open(os.path.join(ROOT, "Makefile"), encoding="utf-8").read()
    targets = set(MAKE_TARGET.findall(makefile))
    for path in files:
        if not path.endswith(".md") or path.startswith(".agents/skills/"):
            continue
        text = open(os.path.join(ROOT, path), encoding="utf-8").read()
        for block in COMMAND_BLOCK.findall(text):
            for target in MAKE_CALL.findall(block):
                if target not in targets:
                    problems.append(f"{path}: make {target} 不在 Makefile 里")
            for command in MYGO_CALL.findall(block):
                if command not in MYGO_COMMANDS:
                    problems.append(
                        f"{path}: go tool mygo {command} 不是已知子命令；"
                        "若 MyGo 新增了命令，先更新 MYGO_COMMANDS"
                    )
            for script in LOCAL_SCRIPT.findall(block):
                if not os.path.exists(os.path.join(ROOT, script)):
                    problems.append(f"{path}: 命令引用的文件不存在: {script}")
    return problems


def check_dependency_direction(files: list[str]) -> list[str]:
    """apps/ 之间互不导入，packages/ 不反向依赖 apps/。"""
    problems = []
    for path in files:
        if not path.endswith(".go"):
            continue
        parts = path.split("/")
        if len(parts) < 3 or parts[0] not in {"apps", "packages"}:
            continue
        source = open(os.path.join(ROOT, path), encoding="utf-8").read()
        for line_no, line in enumerate(source.splitlines(), 1):
            for imported in re.findall(r'"(luvia/[^"]+)"', line):
                if parts[0] == "apps":
                    segments = imported.split("/")
                    if len(segments) > 2 and segments[1] == "apps" and segments[2] != parts[1]:
                        problems.append(
                            f"{path}:{line_no}: 导入 {imported}，应用之间不互相依赖"
                        )
                elif imported.startswith("luvia/apps/"):
                    problems.append(
                        f"{path}:{line_no}: 导入 {imported}，共享模块不得反向依赖应用"
                    )
    return problems


def check_frontend_files(files: list[str]) -> list[str]:
    """这个项目的界面是 Go + 原生控件，不引入前端工程链或 WebView。"""
    problems = []
    for path in files:
        if not path.startswith("apps/"):
            continue
        name = os.path.basename(path)
        if name in FRONTEND_FILES or path.endswith(FRONTEND_EXT):
            problems.append(f"{path}: 前端工程文件；界面用 MyGo 的原生控件实现")
            continue
        if not path.endswith(".go"):
            continue
        for line_no, line in enumerate(open(os.path.join(ROOT, path), encoding="utf-8"), 1):
            code = line.split("//", 1)[0]
            for token in WEBVIEW_TOKENS:
                if token in code:
                    problems.append(f"{path}:{line_no}: 出现 {token}，不允许 WebView 与前端绑定")
    return problems


def check_secrets(files: list[str]) -> list[str]:
    problems = []
    for path in files:
        for pattern in SECRET_FILES:
            if re.search(pattern, path):
                problems.append(f"{path}: 密钥文件不进版本库")
        if path.endswith((".png", ".webp", ".icns", ".car", ".zip", ".dmg")):
            continue
        try:
            text = open(os.path.join(ROOT, path), encoding="utf-8").read()
        except (OSError, UnicodeDecodeError):
            continue
        for pattern, why in SECRETS:
            if re.search(pattern, text):
                problems.append(f"{path}: 疑似{why}，凭证不进版本库")
    return problems


def check_local_paths(files: list[str]) -> list[str]:
    """文档与脚本里写死本机路径，换一台机器就失效。"""
    problems = []
    for path in files:
        if path.startswith(".agents/skills/") or path == "scripts/check-rules.py":
            continue
        if not path.endswith((".md", ".py", ".sh", ".yml", ".yaml", ".json", ".go", ".toml")):
            continue
        for line_no, line in enumerate(open(os.path.join(ROOT, path), encoding="utf-8"), 1):
            match = LOCAL_PATH.search(line)
            if match:
                problems.append(
                    f"{path}:{line_no}: 写死了本机路径 {match.group(1)}，改用仓库相对路径或临时目录"
                )
    return problems


def check_sizes(files: list[str]) -> list[str]:
    problems = []
    for path in files:
        full = os.path.join(ROOT, path)
        size = os.path.getsize(full)
        if size > MAX_FILE_BYTES and not path.startswith("assets/"):
            problems.append(f"{path}: {size // 1024} KB，大文件应放 assets/ 或仓库之外")
        if re.match(r"^apps/[^/]+/main\.go$", path):
            lines = open(full, encoding="utf-8").read().count("\n")
            if lines > MAX_ENTRY_LINES:
                problems.append(
                    f"{path}: {lines} 行，入口只负责组装；把功能拆到独立文件"
                )
    return problems


def check_text_hygiene(files: list[str]) -> list[str]:
    problems = []
    for path in files:
        try:
            with open(os.path.join(ROOT, path), "rb") as source:
                raw = source.read()
        except OSError:
            continue
        if b"\x00" in raw:
            continue
        if b"\r\n" in raw:
            problems.append(f"{path}: 使用 CRLF，统一为 LF")
        # Preserve the checksum-managed upstream SVG bytes. make check also
        # runs icons-check to enforce manifest membership and SHA-256.
        managed_svg = re.fullmatch(r"assets/ui-icons/solar/solar--[a-z0-9-]+\.svg", path)
        if raw and not raw.endswith(b"\n") and not managed_svg:
            problems.append(f"{path}: 缺少结尾换行")
        if path.endswith((".md", ".png", ".webp")):
            continue
        for line_no, line in enumerate(raw.split(b"\n"), 1):
            if line != line.rstrip(b" \t"):
                problems.append(f"{path}:{line_no}: 行尾有多余空白")
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
