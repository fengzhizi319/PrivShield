// Package microservices provides transparent HTTP proxy clients from the
// main console/bff-go gateway to the three Go microservices:
// service-hub, datasource-mgr, and audit-log.
//
// It is intentionally thin: it forwards method/path/query/body as-is and only
// injects the trace/auth headers required for zero-trust outbound calls.
package microservices

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/fengzhizi319/PrivShield/console/bff-go/internal/config"
	pkgagent "github.com/fengzhizi319/PrivShield/pkg/agent"
)

// ClientPool forwards HTTP requests to the Go microservices with unified
// trace and auth header injection.
type ClientPool struct {
	httpClient *http.Client
	urls       map[string]string
	apiKeys    map[string]string
}

// NewClientPool creates a microservice proxy pool from config.
func NewClientPool(cfg *config.Config) *ClientPool {
	return &ClientPool{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 25,
				IdleConnTimeout:     90 * time.Second,
			},
		},
		urls: map[string]string{
			"hub":        normalizeBaseURL(cfg.HubURL),
			"datasource": normalizeBaseURL(cfg.DatasourceURL),
			"audit":      normalizeBaseURL(cfg.AuditURL),
		},
		apiKeys: map[string]string{
			"hub":        cfg.HubAPIKey,
			"datasource": cfg.DatasourceAPIKey,
			"audit":      cfg.AuditAPIKey,
		},
	}
}

// Proxy forwards a request to the named service and returns the upstream
// response status, body, and an error if the round-trip itself failed.
func (p *ClientPool) Proxy(ctx context.Context, service, method, path string, query url.Values, body []byte, contentType, requestID string) (*http.Response, []byte, error) {
	base, ok := p.urls[service]
	if !ok {
		return nil, nil, fmt.Errorf("unknown microservice: %s", service)
	}

	u, err := url.Parse(base + path)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid target URL: %w", err)
	}
	if query != nil {
		u.RawQuery = query.Encode()
	}

	var bodyReader io.Reader
	if len(body) > 0 {
		bodyReader = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, u.String(), bodyReader)
	if err != nil {
		return nil, nil, fmt.Errorf("create request: %w", err)
	}

	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	// Inject trace headers (dual-header propagation aligned with pkg/agent).
	if requestID != "" {
		req.Header.Set("X-Request-ID", requestID)
		req.Header.Set("X-Trace-ID", requestID)
	} else if rid := pkgagent.RequestIDFromContext(ctx); rid != "" {
		req.Header.Set("X-Request-ID", rid)
		req.Header.Set("X-Trace-ID", rid)
	}
	// Inject service-to-service API key if configured.
	if key := p.apiKeys[service]; key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("%s request failed: %w", service, err)
	}
	defer resp.Body.Close()

	const maxBodySize = 64 << 20 // 64 MiB
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxBodySize+1))
	if err != nil {
		return nil, nil, fmt.Errorf("read %s response: %w", service, err)
	}
	if int64(len(respBody)) > maxBodySize {
		return nil, nil, fmt.Errorf("%s response exceeded %d MB limit", service, maxBodySize>>20)
	}
	return resp, respBody, nil
}

// normalizeBaseURL trims trailing slashes so path concatenation is predictable.
func normalizeBaseURL(u string) string {
	return strings.TrimRight(u, "/")
}
