package tlsutil

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

// tlsPeerContext creates a context with a mock TLS peer containing the given CN.
// tlsPeerContext 构造一个携带模拟 TLS peer（含指定 CN）的 context。
func tlsPeerContext(cn string) context.Context {
	return peer.NewContext(context.Background(), &peer.Peer{
		AuthInfo: credentials.TLSInfo{
			State: tls.ConnectionState{
				VerifiedChains: [][]*x509.Certificate{
					{{Subject: pkix.Name{CommonName: cn}}},
				},
			},
		},
	})
}

// TestExtractClientCN_Valid verifies extraction of CN from a valid TLS peer context.
func TestExtractClientCN_Valid(t *testing.T) {
	ctx := tlsPeerContext("test-client.internal")
	cn, err := extractClientCN(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cn != "test-client.internal" {
		t.Errorf("CN = %q, want %q", cn, "test-client.internal")
	}
}

// TestExtractClientCN_NoPeer verifies Unauthenticated error when no peer exists.
func TestExtractClientCN_NoPeer(t *testing.T) {
	_, err := extractClientCN(context.Background())
	if err == nil {
		t.Fatal("expected error for missing peer")
	}
	st, ok := status.FromError(err)
	if !ok || st.Code() != codes.Unauthenticated {
		t.Errorf("expected Unauthenticated, got %v", err)
	}
}

// TestExtractClientCN_NoTLS verifies Unauthenticated error when peer has no TLS info.
func TestExtractClientCN_NoTLS(t *testing.T) {
	ctx := peer.NewContext(context.Background(), &peer.Peer{})
	_, err := extractClientCN(ctx)
	if err == nil {
		t.Fatal("expected error for missing TLS info")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.Unauthenticated {
		t.Errorf("expected Unauthenticated, got %v", st.Code())
	}
}

// TestAuthorizeClient_WildcardScope verifies wildcard "*" scope allows any method.
func TestAuthorizeClient_WildcardScope(t *testing.T) {
	path := createTempWhitelistForInterceptor(t)
	dw, err := NewDynamicWhitelist(path)
	if err != nil {
		t.Fatalf("NewDynamicWhitelist failed: %v", err)
	}
	defer dw.Close()

	// bff-go has wildcard scope — should allow any method
	if err := dw.authorizeClient("bff-go.privshield.internal", "/AnyService/AnyMethod"); err != nil {
		t.Errorf("expected wildcard scope to allow any method, got: %v", err)
	}
}

// TestAuthorizeClient_SpecificScope verifies specific method scope enforcement.
func TestAuthorizeClient_SpecificScope(t *testing.T) {
	path := createTempWhitelistForInterceptor(t)
	dw, err := NewDynamicWhitelist(path)
	if err != nil {
		t.Fatalf("NewDynamicWhitelist failed: %v", err)
	}
	defer dw.Close()

	// service-hub has /PrivacyService/Process — should be allowed
	if err := dw.authorizeClient("service-hub.privshield.internal", "/PrivacyService/Process"); err != nil {
		t.Errorf("expected allowed for /PrivacyService/Process, got: %v", err)
	}

	// service-hub has /AuditLog/* — should allow /AuditLog/RecordAudit
	if err := dw.authorizeClient("service-hub.privshield.internal", "/AuditLog/RecordAudit"); err != nil {
		t.Errorf("expected allowed for /AuditLog/RecordAudit via wildcard, got: %v", err)
	}

	// service-hub should NOT have access to /DatasourceMgr/FetchSlice
	err = dw.authorizeClient("service-hub.privshield.internal", "/DatasourceMgr/FetchSlice")
	if err == nil {
		t.Fatal("expected PermissionDenied for unauthorized method")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.PermissionDenied {
		t.Errorf("expected PermissionDenied, got %v", st.Code())
	}
}

// TestAuthorizeClient_UnknownCN verifies unknown CN is rejected.
func TestAuthorizeClient_UnknownCN(t *testing.T) {
	path := createTempWhitelistForInterceptor(t)
	dw, err := NewDynamicWhitelist(path)
	if err != nil {
		t.Fatalf("NewDynamicWhitelist failed: %v", err)
	}
	defer dw.Close()

	err = dw.authorizeClient("unknown-client", "/AnyMethod")
	if err == nil {
		t.Fatal("expected PermissionDenied for unknown CN")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.PermissionDenied {
		t.Errorf("expected PermissionDenied, got %v", st.Code())
	}
}

// TestUnaryServerInterceptor_Authorized verifies the interceptor passes through for authorized CN.
func TestUnaryServerInterceptor_Authorized(t *testing.T) {
	path := createTempWhitelistForInterceptor(t)
	dw, err := NewDynamicWhitelist(path)
	if err != nil {
		t.Fatalf("NewDynamicWhitelist failed: %v", err)
	}
	defer dw.Close()

	interceptor := dw.UnaryServerInterceptor()
	ctx := tlsPeerContext("bff-go.privshield.internal")
	handlerCalled := false

	resp, err := interceptor(ctx, "test-request", &grpc.UnaryServerInfo{FullMethod: "/TestService/TestMethod"}, func(ctx context.Context, req any) (any, error) {
		handlerCalled = true
		return "handler-response", nil
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !handlerCalled {
		t.Error("expected handler to be called")
	}
	if resp != "handler-response" {
		t.Errorf("unexpected response: %v", resp)
	}
}

// TestUnaryServerInterceptor_Unauthorized verifies the interceptor blocks unauthorized CN.
func TestUnaryServerInterceptor_Unauthorized(t *testing.T) {
	path := createTempWhitelistForInterceptor(t)
	dw, err := NewDynamicWhitelist(path)
	if err != nil {
		t.Fatalf("NewDynamicWhitelist failed: %v", err)
	}
	defer dw.Close()

	interceptor := dw.UnaryServerInterceptor()
	ctx := tlsPeerContext("unknown-client")

	_, err = interceptor(ctx, "test-request", &grpc.UnaryServerInfo{FullMethod: "/TestService/TestMethod"}, func(ctx context.Context, req any) (any, error) {
		t.Error("handler should NOT be called for unauthorized CN")
		return nil, nil
	})

	if err == nil {
		t.Fatal("expected error for unauthorized CN")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.PermissionDenied {
		t.Errorf("expected PermissionDenied, got %v", st.Code())
	}
}

// TestUnaryServerInterceptor_ScopeViolation verifies method scope is enforced.
func TestUnaryServerInterceptor_ScopeViolation(t *testing.T) {
	path := createTempWhitelistForInterceptor(t)
	dw, err := NewDynamicWhitelist(path)
	if err != nil {
		t.Fatalf("NewDynamicWhitelist failed: %v", err)
	}
	defer dw.Close()

	interceptor := dw.UnaryServerInterceptor()
	// service-hub is authorized but only for /PrivacyService/Process and /AuditLog/*
	ctx := tlsPeerContext("service-hub.privshield.internal")

	_, err = interceptor(ctx, "test-request", &grpc.UnaryServerInfo{FullMethod: "/DatasourceMgr/FetchSlice"}, func(ctx context.Context, req any) (any, error) {
		t.Error("handler should NOT be called for out-of-scope method")
		return nil, nil
	})

	if err == nil {
		t.Fatal("expected error for out-of-scope method")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.PermissionDenied {
		t.Errorf("expected PermissionDenied, got %v", st.Code())
	}
}

// TestUnaryServerInterceptor_NoTLS verifies the interceptor rejects non-TLS connections.
func TestUnaryServerInterceptor_NoTLS(t *testing.T) {
	path := createTempWhitelistForInterceptor(t)
	dw, err := NewDynamicWhitelist(path)
	if err != nil {
		t.Fatalf("NewDynamicWhitelist failed: %v", err)
	}
	defer dw.Close()

	interceptor := dw.UnaryServerInterceptor()
	// Plain context without TLS peer
	ctx := context.Background()

	_, err = interceptor(ctx, "test-request", &grpc.UnaryServerInfo{FullMethod: "/TestService/TestMethod"}, func(ctx context.Context, req any) (any, error) {
		t.Error("handler should NOT be called without TLS")
		return nil, nil
	})

	if err == nil {
		t.Fatal("expected error for non-TLS connection")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.Unauthenticated {
		t.Errorf("expected Unauthenticated, got %v", st.Code())
	}
}

// createTempWhitelistForInterceptor creates a temp whitelist for interceptor tests.
func createTempWhitelistForInterceptor(t *testing.T) string {
	t.Helper()
	return createTempWhitelist(t, testWhitelistYAML)
}

// TestNewWhitelistInterceptor_EmptyPath verifies that an empty path returns nil interceptors.
func TestNewWhitelistInterceptor_EmptyPath(t *testing.T) {
	unary, stream, dw, err := NewWhitelistInterceptor("")
	if err != nil {
		t.Fatalf("unexpected error for empty path: %v", err)
	}
	if unary != nil {
		t.Error("expected nil unary interceptor for empty path")
	}
	if stream != nil {
		t.Error("expected nil stream interceptor for empty path")
	}
	if dw != nil {
		t.Error("expected nil DynamicWhitelist for empty path")
	}
}

// TestNewWhitelistInterceptor_LoadAndAuthorize verifies the helper loads the whitelist
// and the returned unary interceptor blocks unauthorized CNs.
func TestNewWhitelistInterceptor_LoadAndAuthorize(t *testing.T) {
	path := createTempWhitelistForInterceptor(t)
	unary, stream, dw, err := NewWhitelistInterceptor(path)
	if err != nil {
		t.Fatalf("NewWhitelistInterceptor failed: %v", err)
	}
	defer dw.Close()
	if unary == nil {
		t.Fatal("expected non-nil unary interceptor")
	}
	if stream == nil {
		t.Fatal("expected non-nil stream interceptor")
	}

	ctx := tlsPeerContext("unknown-client")
	_, err = unary(ctx, "test-request", &grpc.UnaryServerInfo{FullMethod: "/TestService/TestMethod"}, func(ctx context.Context, req any) (any, error) {
		t.Error("handler should NOT be called for unauthorized CN")
		return nil, nil
	})
	if err == nil {
		t.Fatal("expected error for unauthorized CN")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.PermissionDenied {
		t.Errorf("expected PermissionDenied, got %v", st.Code())
	}
}
