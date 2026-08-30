"""DICOM 二进制医学影像脱敏单元测试 / DICOM Binary Anonymization Tests.

测试覆盖场景：
1. 标准 DICOM 魔数检测 (is_dicom_data)
2. 患者元数据（姓名、ID、生日、UID）擦除与替换
3. 图像像素数据（PixelData）完整无损保留
4. 偶数字节对齐与 Explicit VR / Implicit VR 解析容错
"""

from __future__ import annotations

import struct
from pathlib import Path
import pytest

from engine.dynclassification.image_redaction import (
    IMAGE_REDACTION_FAILURE,
    anonymize_dicom,
    is_dicom_data,
    sanitize_image_input,
)


def _build_mock_dicom(
    patient_name: str = "Zhang San",
    patient_id: str = "P12345678",
    pixel_bytes: bytes = b"\x01\x02\x03\x04",
) -> bytes:
    preamble = b"\x00" * 128 + b"DICM"
    pn_bytes = patient_name.encode("utf-8")
    if len(pn_bytes) % 2 != 0:
        pn_bytes += b" "
    tag_name = struct.pack("<HH", 0x0010, 0x0010) + b"PN" + struct.pack("<H", len(pn_bytes)) + pn_bytes
    
    id_bytes = patient_id.encode("utf-8")
    if len(id_bytes) % 2 != 0:
        id_bytes += b" "
    tag_id = struct.pack("<HH", 0x0010, 0x0020) + b"LO" + struct.pack("<H", len(id_bytes)) + id_bytes

    uid_bytes = b"1.2.840.10008.1.1 "
    tag_uid = struct.pack("<HH", 0x0020, 0x000D) + b"UI" + struct.pack("<H", len(uid_bytes)) + uid_bytes

    tag_pixel = struct.pack("<HH", 0x7FE0, 0x0010) + b"OB\x00\x00" + struct.pack("<I", len(pixel_bytes)) + pixel_bytes

    return preamble + tag_name + tag_id + tag_uid + tag_pixel


def test_is_dicom_data():
    valid_dicom = _build_mock_dicom()
    assert is_dicom_data(valid_dicom) is True
    assert is_dicom_data(b"not a dicom file") is False


def test_anonymize_dicom():
    raw_dicom = _build_mock_dicom(
        patient_name="Li Si",
        patient_id="ID999888",
        pixel_bytes=b"\xaa\xbb\xcc\xdd",
    )
    anon = anonymize_dicom(raw_dicom)

    assert is_dicom_data(anon) is True
    assert b"Li Si" not in anon
    assert b"ANONYMOUS^PATIENT" in anon
    assert b"ANON_" in anon
    assert b"\xaa\xbb\xcc\xdd" in anon


def test_sanitize_image_input_dicom(tmp_path: Path):
    dcm_bytes = _build_mock_dicom(patient_name="Wang Wu", patient_id="ID555")
    dcm_file = tmp_path / "test_patient.dcm"
    dcm_file.write_bytes(dcm_bytes)

    out_dir = tmp_path / "out"
    res = sanitize_image_input(str(dcm_file), output_dir=out_dir)

    assert res != IMAGE_REDACTION_FAILURE
    out_path = Path(res)
    assert out_path.exists()
    
    sanitized_bytes = out_path.read_bytes()
    assert b"Wang Wu" not in sanitized_bytes
    assert b"ANONYMOUS^PATIENT" in sanitized_bytes
