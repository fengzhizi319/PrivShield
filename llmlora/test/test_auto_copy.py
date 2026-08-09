"""Test auto-copy of merged LoRA model to Agent .models directory."""

from __future__ import annotations

import shutil
from pathlib import Path
from unittest.mock import MagicMock, patch

from llmlora.src.models.trainer import LoRATrainingRunner
from llmlora.src.utils.config import Config


def test_copy_to_agent_model_dir(tmp_path):
    src_dir = tmp_path / "merged_output"
    dst_dir = tmp_path / "agent_models" / "Qwen3.5-0.8B-Privacy-Classifier-Smoother"

    src_dir.mkdir(parents=True, exist_ok=True)
    (src_dir / "config.json").write_text('{"model_type": "qwen3_5"}', encoding="utf-8")
    (src_dir / "model.safetensors").write_text("fake_weights_data", encoding="utf-8")

    sub_dir = src_dir / "sub_folder"
    sub_dir.mkdir()
    (sub_dir / "tokenizer.json").write_text('{"tokenizer": "test"}', encoding="utf-8")

    cfg = Config()
    cfg.merged_output_dir = str(src_dir)
    cfg.agent_model_dir = str(dst_dir)
    cfg.auto_copy_to_agent_dir = True

    runner = LoRATrainingRunner(cfg)
    runner._copy_to_agent_model_dir()

    assert dst_dir.exists()
    assert (dst_dir / "config.json").read_text(encoding="utf-8") == '{"model_type": "qwen3_5"}'
    assert (dst_dir / "model.safetensors").read_text(encoding="utf-8") == "fake_weights_data"
    assert (dst_dir / "sub_folder" / "tokenizer.json").read_text(encoding="utf-8") == '{"tokenizer": "test"}'
