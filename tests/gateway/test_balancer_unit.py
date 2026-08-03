"""负载均衡器单元回归测试（无需真实后端）。

覆盖以下修复点：
- remove_node 在同步上下文（无运行中事件循环）不再因 create_task 崩溃；
- add_node 重新注册既有节点时同步刷新健康节点数 gauge。
"""

from __future__ import annotations

from privacy_local_agent.gateway.balancer import LoadBalancer
from privacy_local_agent.observability.metrics import GATEWAY_HEALTHY_NODES


def test_remove_node_in_sync_context():
    """同步上下文（脚本/测试）中注销节点不应抛 RuntimeError。"""
    balancer = LoadBalancer(strategy="round_robin")
    balancer.add_node("http://127.0.0.1:1", "127.0.0.1:1")
    assert len(balancer.nodes) == 1

    # 无运行中的事件循环，此前 create_task 会抛 RuntimeError
    balancer.remove_node("http://127.0.0.1:1", "127.0.0.1:1")
    assert balancer.nodes == []


def test_readd_existing_node_restores_health_and_gauge():
    """重复注册既有节点应恢复健康状态并刷新健康节点数指标。"""
    balancer = LoadBalancer(strategy="round_robin")
    balancer.add_node("http://127.0.0.1:1", "127.0.0.1:1")
    node = balancer.nodes[0]

    # 模拟节点被被动下线
    node.is_healthy = False
    GATEWAY_HEALTHY_NODES.set(0)

    # 重新注册同一节点 → 恢复健康并更新 gauge
    balancer.add_node("http://127.0.0.1:1", "127.0.0.1:1")
    assert node.is_healthy is True
    assert len(balancer.nodes) == 1
    assert GATEWAY_HEALTHY_NODES._value.get() == 1.0
