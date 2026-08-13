"""scripts/dev/docker-start-agent.sh 启动脚本测试 / docker-start-agent.sh Script Tests.

验证 scripts/dev/docker-start-agent.sh（Docker 方式单组分启动 Privacy Local Agent 容器）
静态检查与 Fake Docker 模拟执行。
"""

from __future__ import annotations

import os
import shutil
import stat
import subprocess
import tempfile
import textwrap
from pathlib import Path
import pytest

PROJECT_ROOT = Path(__file__).resolve().parents[2]
SCRIPT_PATH = PROJECT_ROOT / "scripts" / "dev" / "docker-start-agent.sh"


@pytest.fixture(scope="module")
def bash_bin() -> str:
    bash = shutil.which("bash")
    if bash is None:
        pytest.skip("bash 不可用，需要 WSL/Linux 环境")
    return bash


def _run_script_with_fake_docker(
    bash_bin: str, exit_code: int = 0, target: str = "core"
) -> tuple[subprocess.CompletedProcess, str]:
    with tempfile.TemporaryDirectory(prefix="fake-docker-agent-") as tmp:
        tmp_dir = Path(tmp)
        log_file = tmp_dir / "docker-calls.log"

        fake_docker = tmp_dir / "docker"
        fake_docker.write_text(
            textwrap.dedent(
                f"""\
                #!/usr/bin/env bash
                echo "[FAKE-DOCKER] args: $*" >> "{log_file}"
                echo "[FAKE-DOCKER] cwd: $(pwd)" >> "{log_file}"
                exit {exit_code}
                """
            ),
            encoding="utf-8",
        )
        fake_docker.chmod(
            fake_docker.stat().st_mode | stat.S_IXUSR | stat.S_IXGRP | stat.S_IXOTH
        )

        # 清理由 Agent 环境注入的超长变量，防范 OSError: [Errno 7] Argument list too long
        env = {
            k: v
            for k, v in os.environ.items()
            if not k.startswith("ANTIGRAVITY") and k != "LS_COLORS"
        }
        env["PATH"] = str(tmp_dir) + os.pathsep + os.environ.get("PATH", "")

        cmd = [bash_bin, str(SCRIPT_PATH)]
        if target:
            cmd.append(target)

        result = subprocess.run(
            cmd,
            capture_output=True,
            text=True,
            env=env,
            cwd=PROJECT_ROOT,
            timeout=60,
        )
        return result, log_file.read_text(encoding="utf-8")


class TestAgentScriptStaticChecks:
    """脚本静态检查。"""

    def test_script_file_exists(self):
        assert SCRIPT_PATH.is_file()

    def test_script_is_executable(self):
        mode = SCRIPT_PATH.stat().st_mode
        assert mode & stat.S_IXUSR, "脚本缺少可执行权限位"

    def test_script_shebang(self):
        first_line = SCRIPT_PATH.read_text(encoding="utf-8").splitlines()[0]
        assert first_line == "#!/usr/bin/env bash"

    def test_script_syntax_valid(self, bash_bin):
        env = {
            k: v
            for k, v in os.environ.items()
            if not k.startswith("ANTIGRAVITY") and k != "LS_COLORS"
        }
        result = subprocess.run([bash_bin, "-n", str(SCRIPT_PATH)], capture_output=True, text=True, env=env)
        assert result.returncode == 0, result.stderr

    def test_script_key_commands_present(self):
        content = SCRIPT_PATH.read_text(encoding="utf-8")
        assert "set -euo pipefail" in content
        assert "docker build" in content
        assert "docker run" in content


class TestAgentScriptFakeExecution:
    """使用 fake docker 模拟脚本执行逻辑。"""

    def test_script_runs_default_core(self, bash_bin):
        result, logs = _run_script_with_fake_docker(bash_bin, exit_code=0, target="core")
        assert result.returncode == 0
        assert "Privacy Local Agent (Docker) 已成功启动" in result.stdout
        assert "privacy-local-agent:0.1.0" in logs

    def test_script_runs_ml_target(self, bash_bin):
        result, logs = _run_script_with_fake_docker(bash_bin, exit_code=0, target="ml")
        assert result.returncode == 0
        assert "privacy-local-agent:0.1.0-ml" in logs


class TestAgentStopScript:
    """docker-stop-agent.sh 停止脚本测试。"""

    def test_stop_script_exists_and_valid(self, bash_bin):
        stop_path = PROJECT_ROOT / "scripts" / "dev" / "docker-stop-agent.sh"
        assert stop_path.is_file()
        assert stop_path.stat().st_mode & stat.S_IXUSR
        result = subprocess.run([bash_bin, "-n", str(stop_path)], capture_output=True, text=True)
        assert result.returncode == 0, result.stderr
