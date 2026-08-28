// Package gateway 提供 gRPC 透明流式代理。
//
// 基于 grpc.UnknownServiceHandler + 原始编解码器 (rawCodec) 实现
// 零编解码字节流透传，避免"先反序列化再序列化"的双重开销。
// 配合 P2C-EWMA 负载均衡与三态熔断器，实现 L7 per-RPC 精准调度。
//
// 参考设计文档 §9.4。
package gateway

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// ──────────────────────────────────────────────
// 原始编解码器（透传 protobuf 字节）
// ──────────────────────────────────────────────

// rawCodec 实现 grpc.encoding.Codec 接口，
// 直接透传原始字节而不做 marshal/unmarshal。
type rawCodec struct{}

func (rawCodec) Marshal(v interface{}) ([]byte, error) {
	if b, ok := v.(*[]byte); ok {
		return *b, nil
	}
	return nil, fmt.Errorf("rawCodec: unsupported type %T", v)
}

func (rawCodec) Unmarshal(data []byte, v interface{}) error {
	if b, ok := v.(*[]byte); ok {
		*b = data
		return nil
	}
	return fmt.Errorf("rawCodec: unsupported type %T", v)
}

func (rawCodec) Name() string { return "raw" }

func (rawCodec) String() string { return "raw" }

// ──────────────────────────────────────────────
// gRPC 透明流代理服务器
// ──────────────────────────────────────────────

// GrpcProxyServer gRPC 透明流代理
type GrpcProxyServer struct {
	lb          *LoadBalancer
	connPool    map[string]*grpc.ClientConn
	connPoolMu  sync.RWMutex
	ewmaAlpha   float64
	dialTimeout time.Duration
}

// NewGrpcProxyServer 创建 gRPC 透明流代理
func NewGrpcProxyServer(lb *LoadBalancer) *GrpcProxyServer {
	return &GrpcProxyServer{
		lb:          lb,
		connPool:    make(map[string]*grpc.ClientConn),
		ewmaAlpha:   0.2,
		dialTimeout: 5 * time.Second,
	}
}

// getOrCreateConn 获取或创建到后端的 gRPC 连接（连接池）
func (g *GrpcProxyServer) getOrCreateConn(addr string) (*grpc.ClientConn, error) {
	g.connPoolMu.RLock()
	conn, ok := g.connPool[addr]
	g.connPoolMu.RUnlock()
	if ok && conn.GetState().String() != "SHUTDOWN" {
		return conn, nil
	}

	g.connPoolMu.Lock()
	defer g.connPoolMu.Unlock()

	// 双重检查
	conn, ok = g.connPool[addr]
	if ok && conn.GetState().String() != "SHUTDOWN" {
		return conn, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), g.dialTimeout)
	defer cancel()

	conn, err := grpc.DialContext(ctx, addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(grpc.ForceCodec(rawCodec{})),
	)
	if err != nil {
		return nil, fmt.Errorf("dial backend %s: %w", addr, err)
	}
	g.connPool[addr] = conn
	return conn, nil
}

// TransparentStreamDirector 实现 grpc.StreamHandler，
// 作为 UnknownServiceHandler 回调处理所有未注册的 gRPC 方法。
//
// 流程：
// 1. 从 ServerStream 提取完整方法名
// 2. P2C-EWMA 选择最优后端节点
// 3. 建立到后端的双向流
// 4. 启动双向并发零拷贝字节流转发
// 5. 更新 EWMA 延迟指标与熔断器状态
func (g *GrpcProxyServer) TransparentStreamDirector(srv interface{}, serverStream grpc.ServerStream) error {
	fullMethod, ok := grpc.MethodFromServerStream(serverStream)
	if !ok {
		return status.Errorf(codes.Internal, "failed to get method name")
	}

	// 1. 选择后端节点
	node := g.lb.SelectNode()
	if node == nil {
		return status.Errorf(codes.Unavailable, "no backend agent available")
	}

	if !node.CB.Allow() {
		return status.Errorf(codes.Unavailable, "backend %s circuit breaker open", node.Address)
	}

	// 2. 获取后端连接
	conn, err := g.getOrCreateConn(node.Address)
	if err != nil {
		node.CB.RecordFailure()
		return status.Errorf(codes.Unavailable, "backend connect error: %v", err)
	}

	// 3. 在途计数 + EWMA 追踪
	node.IncrementInFlight()
	start := time.Now()
	defer func() {
		node.DecrementInFlight()
		node.UpdateEWMA(time.Since(start), g.ewmaAlpha)
	}()

	// 4. 建立到后端的客户端流
	ctx := serverStream.Context()
	// 传递 metadata（trace ID 等）
	md, _ := metadata.FromIncomingContext(ctx)
	outCtx := metadata.NewOutgoingContext(ctx, md.Copy())

	clientStream, err := conn.NewStream(outCtx, &grpc.StreamDesc{
		ServerStreams: true,
		ClientStreams: true,
	}, fullMethod)
	if err != nil {
		node.CB.RecordFailure()
		return status.Errorf(codes.Unavailable, "backend stream error: %v", err)
	}

	// 5. 双向零拷贝流转发
	errChan := make(chan error, 2)

	// 客户端 → 后端
	go func() {
		for {
			var frame []byte
			if err := serverStream.RecvMsg(&frame); err != nil {
				if err == io.EOF {
					_ = clientStream.CloseSend()
					errChan <- nil
					return
				}
				errChan <- err
				return
			}
			if err := clientStream.SendMsg(&frame); err != nil {
				errChan <- err
				return
			}
		}
	}()

	// 后端 → 客户端
	go func() {
		for {
			var frame []byte
			if err := clientStream.RecvMsg(&frame); err != nil {
				if err == io.EOF {
					errChan <- nil
					return
				}
				errChan <- err
				return
			}
			if err := serverStream.SendMsg(&frame); err != nil {
				errChan <- err
				return
			}
		}
	}()

	// 等待任一流方向结束
	err = <-errChan
	if err == nil {
		node.CB.RecordSuccess()
	} else {
		slog.Warn("grpc proxy stream error",
			"method", fullMethod,
			"backend", node.Address,
			"error", err.Error(),
		)
		node.CB.RecordFailure()
	}
	return err
}

// Close 关闭所有后端连接
func (g *GrpcProxyServer) Close() error {
	g.connPoolMu.Lock()
	defer g.connPoolMu.Unlock()

	for addr, conn := range g.connPool {
		if err := conn.Close(); err != nil {
			slog.Warn("close backend connection error", "addr", addr, "err", err)
		}
		delete(g.connPool, addr)
	}
	return nil
}

// NewGrpcProxyListener 创建并启动 gRPC 透明流代理服务器
// 返回 grpc.Server 实例用于优雅停机。
func NewGrpcProxyListener(lb *LoadBalancer, listenAddr string) (*grpc.Server, net.Listener, error) {
	lis, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return nil, nil, fmt.Errorf("listen %s: %w", listenAddr, err)
	}

	proxy := NewGrpcProxyServer(lb)

	grpcServer := grpc.NewServer(
		grpc.UnknownServiceHandler(proxy.TransparentStreamDirector),
		grpc.CustomCodec(rawCodec{}),
	)

	return grpcServer, lis, nil
}
