#!/usr/bin/env bash
""":"
# Python 虚拟 Mock 侧边栏服务 (使用 Python 内置 http.server，零额外依赖)
# 允许在无 GPU 硬件或无需拉起重型 ML 模型的无环境开发/CI 容器中，为 Console 模拟完全兼容的 REST 接口。
exec python3 "$0" "$@"
"""

import json
import logging
import sys
from http.server import BaseHTTPRequestHandler, HTTPServer
from typing import Any, Dict

logging.basicConfig(level=logging.INFO, format="%(asctime)s - [MockAgent] - %(levelname)s - %(message)s")

HOST = "127.0.0.1"
PORT = 8079


class MockAgentRequestHandler(BaseHTTPRequestHandler):
    """模拟 PrivShield REST API 的通用 RequestHandler (全覆盖 200 OK)"""

    def _set_headers(self, status_code: int = 200, content_type: str = "application/json"):
        self.send_response(status_code)
        self.send_header("Content-Type", content_type)
        self.send_header("Access-Control-Allow-Origin", "*")
        self.send_header("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
        self.send_header("Access-Control-Allow-Headers", "Content-Type, Authorization")
        self.end_headers()

    def do_OPTIONS(self):
        """处理 CORS 预检请求"""
        self._set_headers(200)

    def do_GET(self):
        """处理所有 GET 请求 (/health, /livez, /readyz, /v1/dynclassification/*)"""
        if self.path in ("/health", "/livez", "/readyz", "/healthz", "/"):
            response = {
                "status": "ok",
                "namespace": "default",
                "version": "0.1.0-mock",
                "uptime_seconds": 100.0,
                "gpu_available": True,
                "mode": "mock_testing"
            }
            self._set_headers(200)
            self.wfile.write(json.dumps(response).encode("utf-8"))
        elif self.path == "/metrics":
            metrics_text = (
                "# HELP pla_requests_total Total HTTP requests\n"
                "# TYPE pla_requests_total counter\n"
                'pla_requests_total{endpoint="/v1/privacy/mask",status="200"} 42\n'
            )
            self._set_headers(200, content_type="text/plain; version=0.0.4")
            self.wfile.write(metrics_text.encode("utf-8"))
        else:
            # 默认给通用 200 OK 字典响应
            response = {"status": "ok", "path": self.path, "data": []}
            self._set_headers(200)
            self.wfile.write(json.dumps(response).encode("utf-8"))

    def do_POST(self):
        """处理所有 POST 脱敏与分类分级模拟请求"""
        content_length = int(self.headers.get("Content-Length", 0))
        body_bytes = self.rfile.read(content_length) if content_length > 0 else b""
        
        try:
            payload = json.loads(body_bytes.decode("utf-8")) if body_bytes else {}
        except Exception:
            payload = {}

        # 针对具体端点返回契约结构的 Mock 响应
        if "mask" in self.path:
            records = payload.get("records", [payload.get("record", {})])
            masked_records = []
            for r in records:
                m_r = dict(r) if isinstance(r, dict) else {"value": r}
                for k in m_r:
                    if isinstance(m_r[k], str):
                        m_r[k] = m_r[k][:2] + "****" if len(m_r[k]) > 2 else "*"
                masked_records.append(m_r)

            response = {
                "success": True,
                "processed_count": len(records),
                "masked_records": masked_records,
                "masked_value": "****",
                "hash": "a1b2c3d4e5f6",
                "result": masked_records[0] if masked_records else {}
            }
        elif "dynclassification" in self.path or "classify" in self.path:
            response = {
                "fieldResult": {"fieldName": payload.get("fieldName", "phone"), "finalLevel": "L3"},
                "category": "PERSONAL_INFO",
                "level": "L3",
                "confidence": 0.99,
                "matched_layer": "Layer2_SmallNER"
            }
        elif "dp" in self.path or "ldp" in self.path:
            response = {
                "noisy_value": 42.0,
                "noisy_values": [1.0, 2.0, 3.0],
                "epsilon_consumed": 0.1,
                "status": "ok"
            }
        else:
            response = {
                "status": "ok",
                "path": self.path,
                "result": "mocked_success"
            }

        self._set_headers(200)
        self.wfile.write(json.dumps(response).encode("utf-8"))

    def log_message(self, format: str, *args: Any):
        """重定向日志输出"""
        logging.info("%s - - [%s]" % (self.address_string(), format % args))


def main():
    port = PORT
    if len(sys.argv) > 1:
        try:
            port = int(sys.argv[1])
        except ValueError:
            pass

    server_address = (HOST, port)
    httpd = HTTPServer(server_address, MockAgentRequestHandler)
    logging.info(f"Mock Agent HTTP 虚拟桩服务启动成功: http://{HOST}:{port}")
    try:
        httpd.serve_forever()
    except KeyboardInterrupt:
        logging.info("收到终止信号，正在关闭 Mock 服务...")
        httpd.server_close()


if __name__ == "__main__":
    main()
