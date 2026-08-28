package gateway

import (
	"context"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func TestRawCodecMarshalUnmarshal(t *testing.T) {
	codec := rawCodec{}

	// Marshal
	data := []byte("hello grpc")
	b, err := codec.Marshal(&data)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if string(b) != "hello grpc" {
		t.Errorf("Marshal = %q, want %q", b, "hello grpc")
	}

	// Unmarshal
	var out []byte
	if err := codec.Unmarshal(b, &out); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if string(out) != "hello grpc" {
		t.Errorf("Unmarshal = %q, want %q", out, "hello grpc")
	}

	// Name
	if codec.Name() != "raw" {
		t.Errorf("Name = %q, want %q", codec.Name(), "raw")
	}
}

func TestRawCodecMarshalTypeError(t *testing.T) {
	codec := rawCodec{}
	_, err := codec.Marshal("not a byte slice")
	if err == nil {
		t.Error("expected error for wrong type")
	}
}

func TestRawCodecUnmarshalTypeError(t *testing.T) {
	codec := rawCodec{}
	err := codec.Unmarshal([]byte("test"), "not a byte slice ptr")
	if err == nil {
		t.Error("expected error for wrong type")
	}
}

func TestNewGrpcProxyServer(t *testing.T) {
	lb := NewLoadBalancer([]string{"127.0.0.1:50051"}, "p2c")
	proxy := NewGrpcProxyServer(lb)
	if proxy == nil {
		t.Fatal("NewGrpcProxyServer returned nil")
	}
	if proxy.ewmaAlpha != 0.2 {
		t.Errorf("ewmaAlpha = %f, want 0.2", proxy.ewmaAlpha)
	}
	if proxy.dialTimeout != 5*time.Second {
		t.Errorf("dialTimeout = %v, want 5s", proxy.dialTimeout)
	}
}

func TestGrpcProxyGetOrCreateConn(t *testing.T) {
	lb := NewLoadBalancer([]string{"127.0.0.1:50051"}, "p2c")
	proxy := NewGrpcProxyServer(lb)
	defer proxy.Close()

	// 第一次创建
	conn, err := proxy.getOrCreateConn("127.0.0.1:50051")
	if err != nil {
		t.Fatalf("getOrCreateConn: %v", err)
	}
	if conn == nil {
		t.Fatal("conn is nil")
	}

	// 第二次应该复用
	conn2, err := proxy.getOrCreateConn("127.0.0.1:50051")
	if err != nil {
		t.Fatalf("getOrCreateConn (cached): %v", err)
	}
	if conn != conn2 {
		t.Error("expected same connection from pool")
	}
}

func TestGrpcProxyClose(t *testing.T) {
	lb := NewLoadBalancer([]string{"127.0.0.1:50051"}, "p2c")
	proxy := NewGrpcProxyServer(lb)

	// 创建连接
	_, _ = proxy.getOrCreateConn("127.0.0.1:50051")

	// 关闭
	if err := proxy.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// 连接池应该为空
	proxy.connPoolMu.RLock()
	count := len(proxy.connPool)
	proxy.connPoolMu.RUnlock()
	if count != 0 {
		t.Errorf("connPool length = %d, want 0", count)
	}
}

func TestNewGrpcProxyListener(t *testing.T) {
	lb := NewLoadBalancer([]string{"127.0.0.1:50051"}, "p2c")

	// 使用 :0 让系统分配端口
	grpcServer, lis, err := NewGrpcProxyListener(lb, "127.0.0.1:0")
	if err != nil {
		t.Fatalf("NewGrpcProxyListener: %v", err)
	}
	if grpcServer == nil || lis == nil {
		t.Fatal("server or listener is nil")
	}

	addr := lis.Addr().String()
	t.Logf("gRPC proxy listening on %s", addr)

	// 启动服务
	go func() {
		_ = grpcServer.Serve(lis)
	}()

	// 等待服务启动
	time.Sleep(50 * time.Millisecond)

	// 尝试连接
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(ctx, addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(grpc.ForceCodec(rawCodec{})),
	)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer conn.Close()

	// 清理
	grpcServer.GracefulStop()
}
