#!/usr/bin/env python3
# -*- coding: utf-8 -*-
r"""
Docs 批量文本查找与替换脚本

用于批量替换 docs 目录中的指定字符串（如 PrivShield -> PrivShield）。
支持特性：
1. 演练模式 (--dry-run)：先预览哪些文件被修改、替换行号及前后 diff，不破坏原文件
2. 备份机制 (--backup)：自动生成 .bak 备份文件
3. 路径自适应：支持 Linux 路径与 WSL 网络路径 (\\wsl.localhost\Ubuntu\...)
4. 多种匹配模式：支持普通文本、正则表达式、大小写忽略、预设多词字典替换
5. 文件过滤：支持自定义扩展名白名单，自动忽略二进制与版本控制目录

使用示例：
  # 1. 预览替换 (Dry Run，不修改文件)
  python scripts/replace_docs_text.py --dry-run

  # 2. 正式执行替换 PrivShield -> PrivShield
  python scripts/replace_docs_text.py

  # 3. 指定路径与目标词 (支持 WSL UNC 路径)
  python scripts/replace_docs_text.py -p docs -f "PrivShield" -r "PrivShield"

  # 4. 替换前自动创建备份文件
  python scripts/replace_docs_text.py --backup
"""

import os
import sys
import re
import argparse
import difflib
from pathlib import Path
from typing import List, Tuple


DEFAULT_EXTENSIONS = [
    ".md", ".markdown", ".txt", ".rst", ".yaml", ".yml", ".json", ".html"
]

IGNORE_DIRS = {
    ".git", ".svn", "__pycache__", ".venv", "venv", "node_modules",
    ".pytest_cache", ".ruff_cache", ".mypy_cache", "site", "dist", "build"
}

PRESET_REPLACEMENTS = {
    "PrivShield": "PrivShield",
}


def normalize_wsl_path(path_str: str) -> Path:
    r"""
    规范化路径，兼容 Windows WSL UNC 路径 (\\wsl.localhost\Ubuntu\...)
    """
    p = path_str.strip()
    
    # 替换 Windows 反斜杠
    normalized = p.replace("\\", "/")
    
    # 匹配 //wsl.localhost/Ubuntu/... 或 //wsl$/Ubuntu/...
    wsl_prefix_pattern = r"^(?://wsl\.localhost/|//wsl\$/)[^/]+(/.*)$"
    match = re.match(wsl_prefix_pattern, normalized, flags=re.IGNORECASE)
    if match:
        linux_path = match.group(1)
        return Path(linux_path)
    
    return Path(path_str)


def is_text_file(filepath: Path) -> bool:
    """检查文件是否为文本文件（避免处理二进制文件）"""
    try:
        with open(filepath, "tr", encoding="utf-8") as f:
            f.read(1024)
            return True
    except (UnicodeDecodeError, PermissionError, IsADirectoryError, OSError):
        return False


def collect_files(target_path: Path, extensions: List[str]) -> List[Path]:
    """收集符合条件的目标文件列表"""
    matched_files = []
    if target_path.is_file():
        if is_text_file(target_path):
            matched_files.append(target_path)
        return matched_files

    if not target_path.exists():
        print(f"[错误] 目标路径不存在: {target_path}", file=sys.stderr)
        return []

    ext_set = {e.lower() if e.startswith(".") else f".{e.lower()}" for e in extensions}

    for root, dirs, files in os.walk(target_path):
        # 过滤忽略目录
        dirs[:] = [d for d in dirs if d not in IGNORE_DIRS]
        
        for file in sorted(files):
            file_path = Path(root) / file
            if file_path.suffix.lower() in ext_set:
                if is_text_file(file_path):
                    matched_files.append(file_path)
                    
    return matched_files


def replace_content(
    content: str,
    replacements: List[Tuple[str, str, bool, bool]]
) -> Tuple[str, int]:
    """
    在文本中执行查找与替换
    replacements: List of (find_pattern, replace_str, is_regex, ignore_case)
    返回: (新文本, 替换次数)
    """
    total_count = 0
    new_content = content

    for find_pat, rep_str, is_regex, ignore_case in replacements:
        flags = re.IGNORECASE if ignore_case else 0
        if is_regex:
            pattern = re.compile(find_pat, flags=flags)
            new_content, count = pattern.subn(rep_str, new_content)
            total_count += count
        else:
            if ignore_case:
                pattern = re.compile(re.escape(find_pat), flags=re.IGNORECASE)
                new_content, count = pattern.subn(rep_str, new_content)
                total_count += count
            else:
                count = new_content.count(find_pat)
                if count > 0:
                    new_content = new_content.replace(find_pat, rep_str)
                    total_count += count

    return new_content, total_count


def show_diff(orig_text: str, new_text: str, file_path: Path):
    """打印彩色或结构化的 diff 差异"""
    orig_lines = orig_text.splitlines(keepends=True)
    new_lines = new_text.splitlines(keepends=True)
    diff = list(difflib.unified_diff(
        orig_lines,
        new_lines,
        fromfile=f"a/{file_path.name}",
        tofile=f"b/{file_path.name}",
        n=2
    ))
    for line in diff:
        if line.startswith("+") and not line.startswith("+++"):
            print(f"\033[32m{line.rstrip()}\033[0m")
        elif line.startswith("-") and not line.startswith("---"):
            print(f"\033[31m{line.rstrip()}\033[0m")
        elif line.startswith("@@"):
            print(f"\033[36m{line.rstrip()}\033[0m")
        else:
            print(line.rstrip())


def main():
    parser = argparse.ArgumentParser(
        description="Docs 批量文本查找与替换工具 (支持 dry-run、备份与 diff)",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog=r"""
示例:
  # 演练预览 (Dry Run)
  python scripts/replace_docs_text.py --dry-run
  
  # 执行替换默认规则 (PrivShield -> PrivShield)
  python scripts/replace_docs_text.py
  
  # 自定义查找与替换词
  python scripts/replace_docs_text.py -f "PrivShield" -r "PrivShield"
  
  # 指定 Windows/WSL 路径或子目录，并在修改前备份
  python scripts/replace_docs_text.py -p "\\wsl.localhost\Ubuntu\home\charles\code\sfwork\PrivShield\docs" --backup
        """
    )
    
    # 默认路径计算：当前目录下的 docs 文件夹
    default_docs = Path(__file__).resolve().parent.parent / "docs"
    if not default_docs.exists():
        default_docs = Path("docs")

    parser.add_argument(
        "-p", "--path",
        default=str(default_docs),
        help=f"目标文件夹或文件路径 (默认: {default_docs})"
    )
    parser.add_argument(
        "-f", "--find",
        default="PrivShield",
        help="要查找的字符串 (默认: 'PrivShield')"
    )
    parser.add_argument(
        "-r", "--replace",
        default="PrivShield",
        help="要替换为的字符串 (默认: 'PrivShield')"
    )
    parser.add_argument(
        "--regex",
        action="store_true",
        help="将查找字符串视为正则表达式"
    )
    parser.add_argument(
        "-i", "--ignore-case",
        action="store_true",
        help="忽略大小写进行匹配"
    )
    parser.add_argument(
        "--preset",
        action="store_true",
        help="使用预设的批量替换字典 (如 PrivShield -> PrivShield)"
    )
    parser.add_argument(
        "-e", "--ext",
        default=",".join(DEFAULT_EXTENSIONS),
        help=f"要处理的文件扩展名列表，逗号分隔 (默认: {','.join(DEFAULT_EXTENSIONS)})"
    )
    parser.add_argument(
        "-d", "--dry-run",
        action="store_true",
        help="演练模式：仅扫描并打印将要修改的内容与 diff，不写回文件"
    )
    parser.add_argument(
        "-b", "--backup",
        action="store_true",
        help="在修改前为原文件创建 .bak 备份"
    )
    parser.add_argument(
        "-v", "--verbose",
        action="store_true",
        help="显示每个文件的修改差异 (diff)"
    )

    args = parser.parse_args()

    # 处理路径
    target_path = normalize_wsl_path(args.path)
    extensions = [ext.strip() for ext in args.ext.split(",") if ext.strip()]

    # 构建替换规则
    replacements = []
    if args.preset:
        for k, v in PRESET_REPLACEMENTS.items():
            replacements.append((k, v, False, args.ignore_case))
    else:
        replacements.append((args.find, args.replace, args.regex, args.ignore_case))

    print("=" * 60)
    print(" 🛠️  PrivShield Docs 批量替换工具")
    print("=" * 60)
    print(f"目标路径: {target_path}")
    print(f"模式:     {'🔍 演练模式 (DRY RUN - 不写回文件)' if args.dry_run else '✍️  执行修改模式'}")
    print(f"备份:     {'启用 (.bak)' if args.backup and not args.dry_run else '禁用'}")
    print("替换规则:")
    for f_pat, r_pat, is_reg, ic in replacements:
        reg_info = " (Regex)" if is_reg else ""
        ic_info = " [忽略大小写]" if ic else ""
        print(f"  - '{f_pat}' -> '{r_pat}'{reg_info}{ic_info}")
    print("=" * 60)

    files = collect_files(target_path, extensions)
    if not files:
        print("未找到需要扫描的文件。")
        return

    print(f"共扫描 {len(files)} 个文本文件...")
    
    modified_files_count = 0
    total_replacements_count = 0

    for file_path in files:
        try:
            with open(file_path, "r", encoding="utf-8") as fp:
                original_content = fp.read()
        except Exception as e:
            print(f"[警告] 无法读取文件 {file_path}: {e}")
            continue

        new_content, count = replace_content(original_content, replacements)

        if count > 0:
            modified_files_count += 1
            total_replacements_count += count
            rel_path = file_path
            try:
                rel_path = file_path.relative_to(target_path)
            except ValueError:
                pass

            print(f"\n📄 [{modified_files_count}] 发现匹配: {rel_path} ({count} 处替换)")

            if args.verbose or args.dry_run:
                show_diff(original_content, new_content, file_path)

            if not args.dry_run:
                if args.backup:
                    bak_path = file_path.with_suffix(file_path.suffix + ".bak")
                    try:
                        with open(bak_path, "w", encoding="utf-8") as bp:
                            bp.write(original_content)
                    except Exception as e:
                        print(f"[错误] 备份失败 {bak_path}: {e}", file=sys.stderr)
                        continue

                try:
                    with open(file_path, "w", encoding="utf-8") as fp:
                        fp.write(new_content)
                except Exception as e:
                    print(f"[错误] 写入失败 {file_path}: {e}", file=sys.stderr)

    print("\n" + "=" * 60)
    print(" 📊 执行统计结果")
    print("=" * 60)
    print(f"扫描文件总数: {len(files)}")
    print(f"命中文件数量: {modified_files_count}")
    print(f"完成替换总数: {total_replacements_count}")

    if args.dry_run:
        print("\n💡 提示: 当前为 --dry-run 演练模式，文件未被修改。")
        print("   确认无误后，去掉 --dry-run 重新运行即可完成实际替换。")
    else:
        if modified_files_count > 0:
            print("\n✅ 所有文件替换已成功完成！")
        else:
            print("\n✨ 没有在文档中发现匹配的待替换文本。")
    print("=" * 60)


if __name__ == "__main__":
    main()
