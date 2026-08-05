"""图像病例与医学影像智能抹平/打码处理模块。
Image Case & Medical Image Smart Redaction / Sanitization Module.
"""

from __future__ import annotations

import base64
from io import BytesIO
import os
from pathlib import Path
from typing import Optional

from ..observability.logging_config import get_logger

logger = get_logger(__name__)


def sanitize_image_input(
    val_str: str,
    output_dir: str | Path = "data/sanitized_images",
    boxes: Optional[list[tuple[float, float, float, float]]] = None,
) -> str:
    """对图片病例（文件路径或 Base64 Data URI）进行图像盲区遮挡与敏感区域智能抹平，保持出参类型与入参一致。
    
    Args:
        val_str: 图片病例入参（图片文件路径，或 Base64 Data URI/字节流）。
        output_dir: 脱敏处理后图片的保存目录。
        boxes: 自定义遮挡框坐标 [(ymin, xmin, ymax, xmax)]，取值范围 0.0 ~ 1.0 比例或像素值。
        
    Returns:
        若入参为文件路径，返回脱敏保存后的新图片文件路径；
        若入参为 Base64 Data URI，返回抹平处理后的 Base64 Data URI；
        若解析失败或非图像，返回原字符串。
    """
    if not val_str:
        return val_str

    try:
        from PIL import Image, ImageDraw
    except ImportError:
        logger.warning("PIL (Pillow) 未安装，无法执行图像病例盲区打码脱敏，退回原值")
        return val_str

    val_stripped = val_str.strip()
    is_data_uri = val_stripped.lower().startswith("data:image/")
    is_file_path = (
        len(val_stripped) < 512
        and any(val_stripped.lower().endswith(ext) for ext in (".jpg", ".jpeg", ".png", ".bmp", ".webp", ".dcm", ".dicom"))
    )

    img: Optional[Image.Image] = None
    file_path_obj: Optional[Path] = None

    # 1. 尝试从文件路径或 Base64 加载 PIL Image
    if is_file_path and os.path.exists(val_stripped):
        file_path_obj = Path(val_stripped)
        try:
            img = Image.open(file_path_obj).convert("RGB")
        except Exception as e:
            logger.warning(f"无法打开图片文件 '{val_stripped}': {e}")
            return val_str
    elif is_data_uri:
        try:
            parts = val_stripped.split(",", 1)
            if len(parts) == 2:
                img_bytes = base64.b64decode(parts[1])
                img = Image.open(BytesIO(img_bytes)).convert("RGB")
        except Exception as e:
            logger.warning(f"无法解码 Base64 图像: {e}")
            return val_str

    if img is None:
        # 非图像格式，直接返回原字符串
        return val_str

    width, height = img.size
    draw = ImageDraw.Draw(img)

    # 2. 图像抹平绘制：若未传入指定框，默认打蔽图像头部 PII 身份区 (0~16% 高度) 与底部高敏病史诊断区 (82%~100% 高度)
    default_boxes = [
        (0.0, 0.0, 1.0, 0.16),    # 头部个人身份姓名/身份证遮挡区
        (0.0, 0.82, 1.0, 1.0),    # 底部敏感诊断/签名遮挡区
    ]
    target_boxes = boxes if boxes else default_boxes

    for box in target_boxes:
        ymin, xmin, ymax, xmax = box
        # 判断是比例坐标 [0.0~1.0] 还是像素坐标
        if all(0.0 <= c <= 1.0 for c in (ymin, xmin, ymax, xmax)):
            x0 = int(xmin * width)
            y0 = int(ymin * height)
            x1 = int(xmax * width)
            y1 = int(ymax * height)
        else:
            x0, y0, x1, y1 = int(xmin), int(ymin), int(xmax), int(ymax)

        # 绘制黑色遮挡框 (Solid Blackout Box)
        draw.rectangle([x0, y0, x1, y1], fill=(0, 0, 0))

    # 3. 输出格式对齐：按入参类型返回相同格式的出参
    if is_file_path and file_path_obj is not None:
        out_dir = Path(output_dir)
        out_dir.mkdir(parents=True, exist_ok=True)
        out_file = out_dir / f"sanitized_{file_path_obj.name}"
        # 兼容后缀格式保存
        fmt = file_path_obj.suffix.lstrip(".").upper()
        if fmt == "JPG":
            fmt = "JPEG"
        if not fmt:
            fmt = "PNG"
        img.save(out_file, format=fmt)
        return str(out_file)
    elif is_data_uri:
        buf = BytesIO()
        img.save(buf, format="PNG")
        b64_str = base64.b64encode(buf.getvalue()).decode("utf-8")
        return f"data:image/png;base64,{b64_str}"

    return val_str
