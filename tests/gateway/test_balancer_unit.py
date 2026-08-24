"""负载均衡器核心组件单元测试 (Unit tests for balancer, circuit breaker & node models).

覆盖以下核心模块与算法逻辑：
1. **CircuitBreaker（熔断器状态机）**：
   - 连续失败达到阈值时由 closed 变为 open 并记录开启时间；
   - 处于 open 期间 allow_request 返回 False；
   - 超过 recovery_timeout 冷却时间后自动转换为 half_open 并允许探测；
   - 在 half_open 状态下探测成功复位为 closed，探测失败重新触发 open 熔断；
2. **BackendNode（工作节点模型）**：
   - 地址正规化与初始属性赋值；
   - track_connection 异步上下文管理器连接数原子增减与异常安全；
   - 双检锁延迟初始化 gRPC 通道与安全关闭；
3. **LoadBalancer（调度策略）**：
   - 轮询 (Round-Robin) 循环调度；
   - 平滑加权轮询 (Smooth Weighted Round-Robin, SWRR) 数学序列精确验证（如 5:1 权重生成 A,A,A,B,A,A）；
   - 最小连接数 (Least Connections) 在途并发敏感选路；
   - 加权随机 (Weighted Random) 调度；
   - 无可用节点或全部节点不健康时返回 None；
   - get_healthy_nodes 3 阶段硬性过滤（主动健康、被动冷却、熔断状态）；
   - remove_node 在同步与异步上下文安全析构；
   - add_node 重复注册幂等更新与指标同步；
4. **health_check_loop（主动健康探针）**：
   - 双协议 HTTP + gRPC 成功时判定健康并复位熔断；
   - 单协议失败时判定不健康并记录熔断失败。
"""

from __future__ import annotations

import asyncio
import time
from unittest.mock import AsyncMock, MagicMock

import pytest

from engine.gateway.balancer import (
    BackendNode,
    CircuitBreaker,
    LoadBalancer,
    health_check_loop,
)
from engine.observability.metrics import GATEWAY_HEALTHY_NODES


# ===========================================================================
# 1. CircuitBreaker 熔断器状态机测试
# ===========================================================================


class TestCircuitBreaker:
    def test_initial_state_is_closed(self):
        cb = CircuitBreaker(failure_threshold=3, recovery_timeout=10.0)
        assert cb.state == "closed"
        assert cb.allow_request() is True
        assert cb._failure_count == 0

    def test_transitions_to_open_after_reaching_failure_threshold(self):
        cb = CircuitBreaker(failure_threshold=3, recovery_timeout=10.0)
        cb.record_failure()
        assert cb.state == "closed"
        assert cb._failure_count == 1
        assert cb.allow_request() is True

        cb.record_failure()
        assert cb.state == "closed"
        assert cb._failure_count == 2
        assert cb.allow_request() is True

        # 达到阈值 3 -> 触发熔断
        cb.record_failure()
        assert cb.state == "open"
        assert cb.allow_request() is False

    def test_transitions_from_open_to_half_open_after_timeout(self, monkeypatch):
        cb = CircuitBreaker(failure_threshold=2, recovery_timeout=5.0)
        cb.record_failure()
        cb.record_failure()
        assert cb.state == "open"
        assert cb.allow_request() is False

        # 模拟时间流逝但未达到恢复窗口
        fake_time = cb._opened_at + 3.0
        monkeypatch.setattr(time, "monotonic", lambda: fake_time)
        assert cb.state == "open"
        assert cb.allow_request() is False

        # 模拟时间流逝超过 recovery_timeout (5.0s)
        fake_time = cb._opened_at + 5.1
        monkeypatch.setattr(time, "monotonic", lambda: fake_time)
        assert cb.state == "half_open"
        assert cb.allow_request() is True

    def test_half_open_success_resets_to_closed(self, monkeypatch):
        cb = CircuitBreaker(failure_threshold=2, recovery_timeout=5.0)
        cb.record_failure()
        cb.record_failure()

        # 进入 half_open
        fake_time = cb._opened_at + 6.0
        monkeypatch.setattr(time, "monotonic", lambda: fake_time)
        assert cb.state == "half_open"

        # 试探成功 -> 完全复位为 closed
        cb.record_success()
        assert cb.state == "closed"
        assert cb._failure_count == 0
        assert cb.allow_request() is True

    def test_half_open_failure_trips_back_to_open(self, monkeypatch):
        cb = CircuitBreaker(failure_threshold=2, recovery_timeout=5.0)
        cb.record_failure()
        cb.record_failure()

        # 进入 half_open
        fake_time = cb._opened_at + 6.0
        monkeypatch.setattr(time, "monotonic", lambda: fake_time)
        assert cb.state == "half_open"

        # 试探再次失败 -> 重新进入 open
        cb.record_failure()
        assert cb.state == "open"
        assert cb.allow_request() is False


# ===========================================================================
# 2. BackendNode 工作节点模型测试
# ===========================================================================


class TestBackendNode:
    def test_initialization_and_url_normalization(self):
        node = BackendNode(http_url="http://127.0.0.1:8079///", grpc_address="127.0.0.1:50051", weight=3)
        assert node.http_url == "http://127.0.0.1:8079"
        assert node.grpc_address == "127.0.0.1:50051"
        assert node.weight == 3
        assert node.current_weight == 0
        assert node.is_healthy is True
        assert node.active_connections == 0
        assert isinstance(node.circuit_breaker, CircuitBreaker)

    def test_weight_minimum_floor(self):
        node = BackendNode(http_url="http://127.0.0.1:8079", grpc_address="127.0.0.1:50051", weight=0)
        assert node.weight == 1

    @pytest.mark.anyio
    async def test_track_connection_context_manager(self):
        node = BackendNode(http_url="http://127.0.0.1:8079", grpc_address="127.0.0.1:50051")
        assert node.active_connections == 0

        async with node.track_connection():
            assert node.active_connections == 1
            async with node.track_connection():
                assert node.active_connections == 2
            assert node.active_connections == 1

        assert node.active_connections == 0

    @pytest.mark.anyio
    async def test_track_connection_handles_exception_safely(self):
        node = BackendNode(http_url="http://127.0.0.1:8079", grpc_address="127.0.0.1:50051")
        with pytest.raises(ValueError, match="boom"):
            async with node.track_connection():
                assert node.active_connections == 1
                raise ValueError("boom")

        assert node.active_connections == 0

    @pytest.mark.anyio
    async def test_close_releases_channel(self):
        node = BackendNode(http_url="http://127.0.0.1:8079", grpc_address="127.0.0.1:50051")
        mock_channel = AsyncMock()
        node._grpc_channel = mock_channel
        node._grpc_stub = MagicMock()

        await node.close()
        mock_channel.close.assert_awaited_once()
        assert node._grpc_channel is None
        assert node._grpc_stub is None


# ===========================================================================
# 3. LoadBalancer 调度算法与节点池管理测试
# ===========================================================================


class TestLoadBalancer:
    def test_remove_node_in_sync_context(self):
        """同步上下文（脚本/测试）中注销节点不应抛 RuntimeError。"""
        balancer = LoadBalancer(strategy="round_robin")
        balancer.add_node("http://127.0.0.1:1", "127.0.0.1:1")
        assert len(balancer.nodes) == 1

        balancer.remove_node("http://127.0.0.1:1", "127.0.0.1:1")
        assert balancer.nodes == []

    @pytest.mark.anyio
    async def test_remove_node_in_async_context(self):
        """异步上下文中注销节点能够调度异步关闭。"""
        balancer = LoadBalancer(strategy="round_robin")
        balancer.add_node("http://127.0.0.1:1", "127.0.0.1:1")
        assert len(balancer.nodes) == 1

        balancer.remove_node("http://127.0.0.1:1", "127.0.0.1:1")
        assert balancer.nodes == []

    def test_readd_existing_node_restores_health_and_gauge(self):
        """重复注册既有节点应恢复健康状态并刷新健康节点数指标。"""
        balancer = LoadBalancer(strategy="round_robin")
        balancer.add_node("http://127.0.0.1:1", "127.0.0.1:1")
        node = balancer.nodes[0]

        # 模拟节点被被动下线
        node.is_healthy = False
        node.active_connections = 10
        GATEWAY_HEALTHY_NODES.set(0)

        # 重新注册同一节点 -> 恢复健康、清零连接数并更新 gauge
        balancer.add_node("http://127.0.0.1:1", "127.0.0.1:1", weight=4)
        assert node.is_healthy is True
        assert node.weight == 4
        assert node.active_connections == 0
        assert len(balancer.nodes) == 1
        assert GATEWAY_HEALTHY_NODES._value.get() == 1.0

    @pytest.mark.anyio
    async def test_select_node_empty_pool_returns_none(self):
        balancer = LoadBalancer(strategy="round_robin")
        assert await balancer.select_node() is None

    @pytest.mark.anyio
    async def test_select_node_all_unhealthy_returns_none(self):
        balancer = LoadBalancer(strategy="round_robin")
        balancer.add_node("http://127.0.0.1:8079", "127.0.0.1:50051")
        balancer.nodes[0].is_healthy = False
        assert await balancer.select_node() is None

    @pytest.mark.anyio
    async def test_get_healthy_nodes_3_stage_filtering(self, monkeypatch):
        balancer = LoadBalancer()
        balancer.add_node("http://node-1", "127.0.0.1:1")
        balancer.add_node("http://node-2", "127.0.0.1:2")
        balancer.add_node("http://node-3", "127.0.0.1:3")

        # node-1: 正常通过
        # node-2: 处于被动故障 5s 冷却期
        balancer.nodes[1].passive_unhealthy_until = time.monotonic() + 10.0
        # node-3: 熔断器处于 open 状态
        balancer.nodes[2].circuit_breaker._state = "open"
        balancer.nodes[2].circuit_breaker._opened_at = time.monotonic()

        healthy = balancer.get_healthy_nodes()
        assert len(healthy) == 1
        assert healthy[0].http_url == "http://node-1"

    @pytest.mark.anyio
    async def test_round_robin_strategy(self):
        balancer = LoadBalancer(strategy="round_robin")
        balancer.add_node("http://node-1", "127.0.0.1:1")
        balancer.add_node("http://node-2", "127.0.0.1:2")
        balancer.add_node("http://node-3", "127.0.0.1:3")

        selected = [await balancer.select_node() for _ in range(6)]
        urls = [n.http_url for n in selected]
        assert urls == [
            "http://node-1",
            "http://node-2",
            "http://node-3",
            "http://node-1",
            "http://node-2",
            "http://node-3",
        ]

    @pytest.mark.anyio
    async def test_smooth_weighted_round_robin_strategy_5_to_1(self):
        """测试 Nginx Smooth Weighted Round-Robin 算法（权重 5:1 时精确生成 A, A, A, B, A, A）。"""
        balancer = LoadBalancer(strategy="weighted_round_robin")
        balancer.add_node("http://node-A", "127.0.0.1:1", weight=5)
        balancer.add_node("http://node-B", "127.0.0.1:2", weight=1)

        selected = [await balancer.select_node() for _ in range(6)]
        names = [n.http_url.replace("http://node-", "") for n in selected]
        # 数学推导序列应为: A, A, A, B, A, A
        assert names == ["A", "A", "A", "B", "A", "A"]

    @pytest.mark.anyio
    async def test_smooth_weighted_round_robin_strategy_4_2_1(self):
        """测试平滑加权轮询（权重 4:2:1 时平滑交错）。"""
        balancer = LoadBalancer(strategy="weighted_round_robin")
        balancer.add_node("http://node-A", "127.0.0.1:1", weight=4)
        balancer.add_node("http://node-B", "127.0.0.1:2", weight=2)
        balancer.add_node("http://node-C", "127.0.0.1:3", weight=1)

        selected = [await balancer.select_node() for _ in range(7)]
        names = [n.http_url.replace("http://node-", "") for n in selected]
        # 统计频次
        assert names.count("A") == 4
        assert names.count("B") == 2
        assert names.count("C") == 1
        # 验证首个被调度的是权重最大的 A
        assert names[0] == "A"

    @pytest.mark.anyio
    async def test_least_connections_strategy(self):
        balancer = LoadBalancer(strategy="least_connections")
        balancer.add_node("http://node-1", "127.0.0.1:1")
        balancer.add_node("http://node-2", "127.0.0.1:2")
        balancer.add_node("http://node-3", "127.0.0.1:3")

        balancer.nodes[0].active_connections = 5
        balancer.nodes[1].active_connections = 1  # 最小
        balancer.nodes[2].active_connections = 3

        selected = await balancer.select_node()
        assert selected.http_url == "http://node-2"

        # 改变连接数
        balancer.nodes[1].active_connections = 8
        balancer.nodes[2].active_connections = 0  # 当前最小

        selected2 = await balancer.select_node()
        assert selected2.http_url == "http://node-3"

    @pytest.mark.anyio
    async def test_random_and_weighted_random_strategy(self):
        balancer = LoadBalancer(strategy="weighted_random")
        balancer.add_node("http://node-A", "127.0.0.1:1", weight=100)
        balancer.add_node("http://node-B", "127.0.0.1:2", weight=1)

        # 抽样 20 次，高权重 node-A 应该占绝大多数
        selected = [await balancer.select_node() for _ in range(20)]
        urls = [n.http_url for n in selected]
        assert urls.count("http://node-A") >= 15


# ===========================================================================
# 4. health_check_loop 主动探针测试
# ===========================================================================


@pytest.mark.anyio
async def test_health_check_loop_sweep(monkeypatch):
    """测试主动健康检查循环单次扫描与指标更新。"""
    balancer = LoadBalancer()
    balancer.add_node("http://node-good", "127.0.0.1:50051")
    balancer.add_node("http://node-bad", "127.0.0.1:50052")

    # Mock gRPC Stub 的 Health 响应
    mock_good_stub = MagicMock()
    mock_good_stub.Health = AsyncMock(return_value=MagicMock(status="ok"))

    mock_bad_stub = MagicMock()
    mock_bad_stub.Health = AsyncMock(side_effect=Exception("gRPC down"))

    balancer.nodes[0]._grpc_stub = mock_good_stub
    balancer.nodes[1]._grpc_stub = mock_bad_stub

    # Mock HTTP 客户端响应
    import httpx

    def mock_get(url, timeout=2.0):
        if "node-good" in url:
            return httpx.Response(200, json={"status": "ok"})
        return httpx.Response(500, json={"status": "down"})

    mock_client = AsyncMock()
    mock_client.get = AsyncMock(side_effect=mock_get)

    class MockAsyncClientContext:
        async def __aenter__(self):
            return mock_client

        async def __aexit__(self, *args):
            pass

    monkeypatch.setattr("httpx.AsyncClient", lambda **kwargs: MockAsyncClientContext())

    # 启动健康检查任务并在执行一轮后取消
    task = asyncio.create_task(health_check_loop(balancer, interval=0.01))
    await asyncio.sleep(0.05)
    task.cancel()
    with pytest.raises(asyncio.CancelledError):
        await task

    assert balancer.nodes[0].is_healthy is True
    assert balancer.nodes[0].circuit_breaker.state == "closed"

    assert balancer.nodes[1].is_healthy is False
    assert balancer.nodes[1].circuit_breaker._failure_count >= 1
