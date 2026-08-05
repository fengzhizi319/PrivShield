"""图像病例与医学影像智能抹平/打码处理模块。
Image Case & Medical Image Smart Redaction / Sanitization Module.
"""

from __future__ import annotations

import base64
from io import BytesIO
import os
from pathlib import Path
from typing import Optional
import tempfile

from ..observability.logging_config import get_logger

logger = get_logger(__name__)

IMAGE_REDACTION_FAILURE = "[IMAGE-REDACTION-FAILED]"


def _cleanup_old_sanitized_images(output_dir: Path, max_files: int = 200) -> None:
    """自动清理旧的打码图片，防止磁盘空间被满存（Disk Exhaustion 防护）。"""
    try:
        if not output_dir.exists():
            return
        files = sorted(output_dir.glob("sanitized_*"), key=lambda p: p.stat().st_mtime)
        if len(files) > max_files:
            for f in files[:-max_files]:
                try:
                    f.unlink(missing_ok=True)
                except Exception:
                    pass
    except Exception:
        pass


def sanitize_image_input(
    val_str: str,
    output_dir: str | Path = "data/sanitized_images",
    boxes: Optional[list[tuple[float, float, float, float]]] = None,
) -> str:
    """对图片病例（文件路径或 Base64 Data URI）进行图像盲区遮挡与敏感区域智能抹平，保持出参类型与入参一致。
    
    内部优化包含：
    1. with Image.open(...) 上下文管理，100% 避免 OS 文件句柄泄漏；
    2. 超高分辨率自动下采样 (2048x2048 thumbnail)，防止 10000x10000 大图耗尽 RAM/VRAM (OOM 防护)；
    3. 磁盘文件数自动淘汰轮转，防止硬盘空间爆满。
    """
    if not val_str:
        return val_str

    try:
        from PIL import Image, ImageDraw
        # 设置 DecompressionBomb 像素上限保护
        Image.MAX_IMAGE_PIXELS = 25_000_000
    except ImportError:
        logger.warning("PIL (Pillow) 未安装，拒绝输出未脱敏图像")
        return IMAGE_REDACTION_FAILURE

    val_stripped = val_str.strip()
    is_data_uri = val_stripped.lower().startswith("data:image/")
    is_image_marker = val_stripped.lower().startswith("image:")
    is_file_path = (
        len(val_stripped) < 512
        and any(
            val_stripped.lower().endswith(ext)
            for ext in (".jpg", ".jpeg", ".png", ".bmp", ".webp", ".tif", ".tiff", ".dcm", ".dicom")
        )
    )

    img: Optional[Image.Image] = None
    file_path_obj: Optional[Path] = Path(val_stripped) if is_file_path else None

    # 1. 尝试从文件路径或 Base64 安全加载并立即分离文件句柄
    if file_path_obj is not None and os.path.exists(file_path_obj):
        input_path = file_path_obj
        try:
            with Image.open(input_path) as raw_img:
                img = raw_img.convert("RGB")
        except Exception as e:
            logger.warning("无法打开图片文件，拒绝输出未脱敏图像: %s", e)
            return IMAGE_REDACTION_FAILURE
    elif is_data_uri:
        try:
            parts = val_stripped.split(",", 1)
            if len(parts) == 2:
                img_bytes = base64.b64decode(parts[1])
                with Image.open(BytesIO(img_bytes)) as raw_img:
                    img = raw_img.convert("RGB")
        except Exception as e:
            logger.warning("无法解码 Base64 图像，拒绝输出未脱敏图像: %s", e)
            return IMAGE_REDACTION_FAILURE

    if img is None:
        # 只要输入看起来是图片，就不能把未知格式当普通文本返回。
        return IMAGE_REDACTION_FAILURE if is_file_path or is_data_uri or is_image_marker else val_str

    # 2. 超高分辨率内存与显存保护 (OOM Prevention)
    # 若图像分辨率超 2048x2048，自动高质量下采样缩放
    max_dim = 2048
    if img.width > max_dim or img.height > max_dim:
        img.thumbnail((max_dim, max_dim), Image.Resampling.LANCZOS)

    width, height = img.size
    draw = ImageDraw.Draw(img)

    # 3. 图像盲区遮挡绘制
    default_boxes = [
        (0.0, 0.0, 1.0, 0.16),    # 头部个人身份姓名/身份证遮挡区
        (0.0, 0.82, 1.0, 1.0),    # 底部敏感诊断/签名遮挡区
    ]
    target_boxes = boxes if boxes else default_boxes

    for box in target_boxes:
        ymin, xmin, ymax, xmax = box
        if all(0.0 <= c <= 1.0 for c in (ymin, xmin, ymax, xmax)):
            x0 = int(xmin * width)
            y0 = int(ymin * height)
            x1 = int(xmax * width)
            y1 = int(ymax * height)
        else:
            x0, y0, x1, y1 = int(xmin), int(ymin), int(xmax), int(ymax)

        draw.rectangle([x0, y0, x1, y1], fill=(0, 0, 0))

    # 4. 输出格式对齐与磁盘防满清理
    if is_file_path and file_path_obj is not None:
        out_dir = Path(output_dir)
        out_dir.mkdir(parents=True, exist_ok=True)
        _cleanup_old_sanitized_images(out_dir, max_files=200)

        out_file = out_dir / f"sanitized_{file_path_obj.name}"
        if out_file.is_symlink():
            img.close()
            logger.warning("拒绝覆盖符号链接形式的图片输出: %s", out_file.name)
            return IMAGE_REDACTION_FAILURE
        fmt = file_path_obj.suffix.lstrip(".").upper()
        if fmt == "JPG":
            fmt = "JPEG"
        if not fmt:
            fmt = "PNG"
        # 在同一目录创建临时文件并原子替换，避免并发请求读到半成品。
        fd, tmp_name = tempfile.mkstemp(prefix=".sanitized_", suffix=out_file.suffix, dir=out_dir)
        os.close(fd)
        tmp_file = Path(tmp_name)
        try:
            img.save(tmp_file, format=fmt)
            tmp_file.replace(out_file)
            return str(out_file)
        finally:
            img.close()
            tmp_file.unlink(missing_ok=True)
    elif is_data_uri:
        buf = BytesIO()
        img.save(buf, format="PNG")
        img.close()
        b64_str = base64.b64encode(buf.getvalue()).decode("utf-8")
        return f"data:image/png;base64,{b64_str}"

    img.close()
    return val_str
