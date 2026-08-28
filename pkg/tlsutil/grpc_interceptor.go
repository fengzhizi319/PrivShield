// Package tlsutil — gRPC server interceptor for mTLS CN whitelist authorization.
// Package tlsutil — 基于 mTLS CN 白名单的 gRPC 服务端拦截器。
//
// UnaryServerInterceptor / StreamServerInterceptor 从客户端证书的 CN 字段提取身份，
// 对照 DynamicWhitelist 进行权限校验（CN 存在性 + method scope 匹配），
// 实现 gRPC 层面的零信任访问控制。
//
// 与 Python 端 engine/security/auth.py 的 AuthInterceptor 对等。
package tlsutil

import (
	"context"
	"log"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

// extractClientCN extracts the Common Name from the peer's TLS client certificate.
// extractClientCN 从对端 TLS 客户端证书中提取 Common Name。
//
// Returns the CN string and true on success, or an error status and false on failure.
// 成功时返回 CN 字符串和 true，失败时返回错误 status 和 false。
func extractClientCN(ctx context.Context) (string, error) {
	p, ok := peer.FromContext(ctx)
	if !ok || p.AuthInfo == nil {
		return "", status.Error(codes.Unauthenticated, "missing peer authentication info")
	}
	tlsInfo, ok := p.AuthInfo.(credentials.TLSInfo)
	if !ok || len(tlsInfo.State.VerifiedChains) == 0 || len(tlsInfo.State.VerifiedChains[0]) == 0 {
		return "", status.Error(codes.Unauthenticated, "invalid or unverified client certificate")
	}
	return tlsInfo.State.VerifiedChains[0][0].Subject.CommonName, nil
}

// authorizeClient checks CN existence in whitelist and method scope matching.
// authorizeClient 校验 CN 是否存在于白名单中，并检查方法级 scope 匹配。
func (dw *DynamicWhitelist) authorizeClient(clientCN, fullMethod string) error {
	dw.mu.RLock()
	scopes, exists := dw.clients[clientCN]
	dw.mu.RUnlock()

	if !exists {
		log.Printf("[mTLS Auth] Unauthorized Client CN: %s", clientCN)
		return status.Errorf(codes.PermissionDenied, "client CN '%s' is not authorized", clientCN)
	}

	// Scope matching: wildcard "*" or exact match or pattern match
	// 范围匹配：通配符 "*"、精确匹配或模式匹配
	for _, s := range scopes {
		if s == "*" || s == fullMethod || matchScopePattern(s, fullMethod) {
			return nil
		}
	}
	log.Printf("[mTLS Auth] CN %s lacks scope for method %s", clientCN, fullMethod)
	return status.Errorf(codes.PermissionDenied, "client CN '%s' lacks scope for method '%s'", clientCN, fullMethod)
}

// UnaryServerInterceptor returns a gRPC unary server interceptor that enforces
// mTLS CN whitelist authorization on every incoming RPC call.
// UnaryServerInterceptor 返回一个 gRPC 一元服务端拦截器，对每个传入的 RPC 调用
// 执行 mTLS CN 白名单授权校验。
//
// Authorization flow / 授权流程：
//  1. Extract client certificate CN from TLS peer info
//     从 TLS peer 信息中提取客户端证书 CN
//  2. Check CN exists in the dynamic whitelist
//     检查 CN 是否存在于动态白名单中
//  3. Verify the RPC method is within the CN's allowed scopes
//     校验 RPC 方法是否在 CN 的允许范围内
//  4. On success, forward to the actual handler
//     校验通过后转发至实际 handler
func (dw *DynamicWhitelist) UnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		clientCN, err := extractClientCN(ctx)
		if err != nil {
			return nil, err
		}
		if err := dw.authorizeClient(clientCN, info.FullMethod); err != nil {
			return nil, err
		}
		return handler(ctx, req)
	}
}

// StreamServerInterceptor returns a gRPC stream server interceptor that enforces
// mTLS CN whitelist authorization on every incoming streaming RPC call.
// StreamServerInterceptor 返回一个 gRPC 流式服务端拦截器，对每个传入的流式 RPC 调用
// 执行 mTLS CN 白名单授权校验。
func (dw *DynamicWhitelist) StreamServerInterceptor() grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		clientCN, err := extractClientCN(ss.Context())
		if err != nil {
			return err
		}
		if err := dw.authorizeClient(clientCN, info.FullMethod); err != nil {
			return err
		}
		return handler(srv, ss)
	}
}

// NewWhitelistInterceptor loads a DynamicWhitelist from path and returns both
// unary and stream server interceptors. If path is empty, it returns nil
// interceptors and a nil DynamicWhitelist with no error.
func NewWhitelistInterceptor(path string) (grpc.UnaryServerInterceptor, grpc.StreamServerInterceptor, *DynamicWhitelist, error) {
	if path == "" {
		return nil, nil, nil, nil
	}
	dw, err := NewDynamicWhitelist(path)
	if err != nil {
		return nil, nil, nil, err
	}
	return dw.UnaryServerInterceptor(), dw.StreamServerInterceptor(), dw, nil
}
