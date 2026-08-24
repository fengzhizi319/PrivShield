// Package datasource provides a client for communicating with the datasource-mgr module.
// Package datasource 提供与模拟数据源服务 (datasource-mgr) 通信的客户端，支持 HTTP REST 与 gRPC (mTLS) 双协议。
package datasource

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"

	dspb "github.com/fengzhizi319/PrivShield/services/datasource-mgr/proto"
	"github.com/fengzhizi319/PrivShield/services/service-hub/internal/config"
)

// Client handles HTTP/REST and gRPC communication with datasource-mgr.
type Client struct {
	cfg        *config.Config
	baseURL    string
	grpcAddr   string
	httpClient *http.Client

	mu         sync.RWMutex
	grpcConn   *grpc.ClientConn
	grpcClient dspb.DataSourceManagerServiceClient
}

// New creates a new Client instance with optional HTTPS mTLS support.
func New(cfg *config.Config) *Client {
	httpClient := &http.Client{
		Timeout: 10 * time.Second,
	}

	if cfg.TLSEnabled && cfg.TLSCertFile != "" && cfg.TLSKeyFile != "" {
		tlsConfig := &tls.Config{
			MinVersion: tls.VersionTLS13,
		}
		if cert, err := tls.LoadX509KeyPair(cfg.TLSCertFile, cfg.TLSKeyFile); err == nil {
			tlsConfig.Certificates = []tls.Certificate{cert}
		}
		if cfg.TLSCAFile != "" {
			if caPEM, err := os.ReadFile(cfg.TLSCAFile); err == nil {
				caPool := x509.NewCertPool()
				if caPool.AppendCertsFromPEM(caPEM) {
					tlsConfig.RootCAs = caPool
				}
			}
		}
		httpClient.Transport = &http.Transport{
			TLSClientConfig: tlsConfig,
		}
	}

	return &Client{
		cfg:        cfg,
		baseURL:    strings.TrimRight(cfg.DatasourceBaseURL(), "/"),
		grpcAddr:   cfg.DatasourceGRPCAddress(),
		httpClient: httpClient,
	}
}

// Close closes any active gRPC connection.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.grpcConn != nil {
		err := c.grpcConn.Close()
		c.grpcConn = nil
		c.grpcClient = nil
		return err
	}
	return nil
}

// ─────────────────────────────────────────────────────────────
// HTTP REST Methods / HTTP REST 方法
// ─────────────────────────────────────────────────────────────

// Health checks datasource-mgr connectivity via HTTP REST.
func (c *Client) Health(ctx context.Context) (map[string]any, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/health", nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do health request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return result, nil
}

// DataQueryResult represents the query result from datasource-mgr.
type DataQueryResult struct {
	SourceID   string           `json:"source_id"`
	SourceName string           `json:"source_name"`
	Total      int              `json:"total"`
	Limit      int              `json:"limit"`
	Offset     int              `json:"offset"`
	Records    []map[string]any `json:"records"`
	Via        string           `json:"via"`
}

// FetchYibaoData requests mock yibao data (API 1) via HTTP REST.
func (c *Client) FetchYibaoData(ctx context.Context, limit, offset int) (*DataQueryResult, error) {
	return c.fetchEndpoint(ctx, "/api/v1/yibao", limit, offset)
}

// FetchKangyangData requests mock kangyang data (API 2) via HTTP REST.
func (c *Client) FetchKangyangData(ctx context.Context, limit, offset int) (*DataQueryResult, error) {
	return c.fetchEndpoint(ctx, "/api/v1/kangyang", limit, offset)
}

// FetchMockData3 requests mock data 3 (API 3) via HTTP REST.
func (c *Client) FetchMockData3(ctx context.Context, limit, offset int) (*DataQueryResult, error) {
	return c.fetchEndpoint(ctx, "/api/v1/mock3", limit, offset)
}

// FetchMockData4 requests mock data 4 (API 4) via HTTP REST.
func (c *Client) FetchMockData4(ctx context.Context, limit, offset int) (*DataQueryResult, error) {
	return c.fetchEndpoint(ctx, "/api/v1/mock4", limit, offset)
}

// FetchDataBySource dispatches to the appropriate endpoint based on source ID or path via HTTP REST.
func (c *Client) FetchDataBySource(ctx context.Context, sourceID string, limit, offset int) (*DataQueryResult, error) {
	normalized := strings.ToLower(strings.TrimSpace(sourceID))
	switch {
	case strings.Contains(normalized, "yibao") || strings.Contains(normalized, "医保"):
		return c.FetchYibaoData(ctx, limit, offset)
	case strings.Contains(normalized, "kangyang") || strings.Contains(normalized, "康养"):
		return c.FetchKangyangData(ctx, limit, offset)
	case strings.Contains(normalized, "mock3") || strings.Contains(normalized, "政务"):
		return c.FetchMockData3(ctx, limit, offset)
	case strings.Contains(normalized, "mock4") || strings.Contains(normalized, "企业") || strings.Contains(normalized, "金融"):
		return c.FetchMockData4(ctx, limit, offset)
	default:
		return c.fetchEndpoint(ctx, "/api/datasources/"+url.PathEscape(sourceID)+"/query", limit, offset)
	}
}

// ListDataSources fetches the list of mock datasources via HTTP REST.
func (c *Client) ListDataSources(ctx context.Context) (map[string]any, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/datasources", nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do list datasources request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return result, nil
}

// GetDataSource fetches a single datasource by ID via HTTP REST.
func (c *Client) GetDataSource(ctx context.Context, id string) (map[string]any, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/datasources/"+url.PathEscape(id), nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do get datasource request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return result, nil
}

// TestConnection tests datasource connectivity via HTTP REST.
func (c *Client) TestConnection(ctx context.Context, id string) (map[string]any, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/datasources/"+url.PathEscape(id)+"/test", nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do test connection request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return result, nil
}

func (c *Client) fetchEndpoint(ctx context.Context, path string, limit, offset int) (*DataQueryResult, error) {
	u, err := url.Parse(c.baseURL + path)
	if err != nil {
		return nil, fmt.Errorf("parse url: %w", err)
	}
	q := u.Query()
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	if offset > 0 {
		q.Set("offset", strconv.Itoa(offset))
	}
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request %s: %w", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("request %s returned status %d: %s", path, resp.StatusCode, string(bodyBytes))
	}

	var result DataQueryResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response from %s: %w", path, err)
	}
	return &result, nil
}

// ─────────────────────────────────────────────────────────────
// gRPC Methods / gRPC 通信方法
// ─────────────────────────────────────────────────────────────

func (c *Client) getGRPCClient(ctx context.Context) (dspb.DataSourceManagerServiceClient, error) {
	c.mu.RLock()
	if c.grpcClient != nil {
		client := c.grpcClient
		c.mu.RUnlock()
		return client, nil
	}
	c.mu.RUnlock()

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.grpcClient != nil {
		return c.grpcClient, nil
	}

	var dialOpt grpc.DialOption
	if c.cfg != nil && c.cfg.TLSEnabled {
		tlsConfig := &tls.Config{MinVersion: tls.VersionTLS13}
		if c.cfg.TLSCAFile != "" {
			caPEM, err := os.ReadFile(c.cfg.TLSCAFile)
			if err != nil {
				return nil, fmt.Errorf("read ca file: %w", err)
			}
			pool := x509.NewCertPool()
			if !pool.AppendCertsFromPEM(caPEM) {
				return nil, fmt.Errorf("append ca cert failed")
			}
			tlsConfig.RootCAs = pool
		}
		if c.cfg.TLSCertFile != "" && c.cfg.TLSKeyFile != "" {
			cert, err := tls.LoadX509KeyPair(c.cfg.TLSCertFile, c.cfg.TLSKeyFile)
			if err != nil {
				return nil, fmt.Errorf("load keypair: %w", err)
			}
			tlsConfig.Certificates = []tls.Certificate{cert}
		}
		dialOpt = grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig))
	} else {
		dialOpt = grpc.WithTransportCredentials(insecure.NewCredentials())
	}

	conn, err := grpc.DialContext(ctx, c.grpcAddr, dialOpt, grpc.WithBlock())
	if err != nil {
		return nil, fmt.Errorf("dial datasource-mgr gRPC at %s: %w", c.grpcAddr, err)
	}

	c.grpcConn = conn
	c.grpcClient = dspb.NewDataSourceManagerServiceClient(conn)
	return c.grpcClient, nil
}

// HealthGRPC checks datasource-mgr connectivity via gRPC.
func (c *Client) HealthGRPC(ctx context.Context) (*dspb.HealthResponse, error) {
	client, err := c.getGRPCClient(ctx)
	if err != nil {
		return nil, err
	}
	return client.Health(ctx, &dspb.HealthRequest{})
}

// FetchYibaoDataGRPC requests mock yibao data (API 1) via gRPC.
func (c *Client) FetchYibaoDataGRPC(ctx context.Context, limit, offset int) (*DataQueryResult, error) {
	client, err := c.getGRPCClient(ctx)
	if err != nil {
		return nil, err
	}
	resp, err := client.GetYibaoData(ctx, &dspb.DataQueryRequest{
		Limit:  int32(limit),
		Offset: int32(offset),
	})
	if err != nil {
		return nil, fmt.Errorf("grpc GetYibaoData: %w", err)
	}
	return protoToQueryResult(resp), nil
}

// FetchKangyangDataGRPC requests mock kangyang data (API 2) via gRPC.
func (c *Client) FetchKangyangDataGRPC(ctx context.Context, limit, offset int) (*DataQueryResult, error) {
	client, err := c.getGRPCClient(ctx)
	if err != nil {
		return nil, err
	}
	resp, err := client.GetKangyangData(ctx, &dspb.DataQueryRequest{
		Limit:  int32(limit),
		Offset: int32(offset),
	})
	if err != nil {
		return nil, fmt.Errorf("grpc GetKangyangData: %w", err)
	}
	return protoToQueryResult(resp), nil
}

// FetchMockData3GRPC requests mock data 3 (API 3) via gRPC.
func (c *Client) FetchMockData3GRPC(ctx context.Context, limit, offset int) (*DataQueryResult, error) {
	client, err := c.getGRPCClient(ctx)
	if err != nil {
		return nil, err
	}
	resp, err := client.GetMockData3(ctx, &dspb.DataQueryRequest{
		Limit:  int32(limit),
		Offset: int32(offset),
	})
	if err != nil {
		return nil, fmt.Errorf("grpc GetMockData3: %w", err)
	}
	return protoToQueryResult(resp), nil
}

// FetchMockData4GRPC requests mock data 4 (API 4) via gRPC.
func (c *Client) FetchMockData4GRPC(ctx context.Context, limit, offset int) (*DataQueryResult, error) {
	client, err := c.getGRPCClient(ctx)
	if err != nil {
		return nil, err
	}
	resp, err := client.GetMockData4(ctx, &dspb.DataQueryRequest{
		Limit:  int32(limit),
		Offset: int32(offset),
	})
	if err != nil {
		return nil, fmt.Errorf("grpc GetMockData4: %w", err)
	}
	return protoToQueryResult(resp), nil
}

// FetchDataBySourceGRPC requests mock data by source ID via gRPC.
func (c *Client) FetchDataBySourceGRPC(ctx context.Context, sourceID string, limit, offset int) (*DataQueryResult, error) {
	client, err := c.getGRPCClient(ctx)
	if err != nil {
		return nil, err
	}
	resp, err := client.GetDataBySource(ctx, &dspb.SourceDataQueryRequest{
		SourceId: sourceID,
		Limit:    int32(limit),
		Offset:   int32(offset),
	})
	if err != nil {
		return nil, fmt.Errorf("grpc GetDataBySource: %w", err)
	}
	return protoToQueryResult(resp), nil
}

// ListMockSourcesGRPC lists mock sources via gRPC.
func (c *Client) ListMockSourcesGRPC(ctx context.Context) (*dspb.ListMockSourcesResponse, error) {
	client, err := c.getGRPCClient(ctx)
	if err != nil {
		return nil, err
	}
	return client.ListMockSources(ctx, &dspb.ListMockSourcesRequest{})
}

// GetDataSourceGRPC gets datasource details via gRPC.
func (c *Client) GetDataSourceGRPC(ctx context.Context, id string) (*dspb.DataSourceProto, error) {
	client, err := c.getGRPCClient(ctx)
	if err != nil {
		return nil, err
	}
	return client.GetDataSource(ctx, &dspb.GetDataSourceRequest{Id: id})
}

// TestConnectionGRPC tests connection via gRPC.
func (c *Client) TestConnectionGRPC(ctx context.Context, id string) (*dspb.TestConnectionResponse, error) {
	client, err := c.getGRPCClient(ctx)
	if err != nil {
		return nil, err
	}
	return client.TestConnection(ctx, &dspb.TestConnectionRequest{Id: id})
}

func protoToQueryResult(resp *dspb.DataQueryResponse) *DataQueryResult {
	if resp == nil {
		return nil
	}
	records := make([]map[string]any, len(resp.Records))
	for i, r := range resp.Records {
		m := make(map[string]any, len(r.Fields))
		for k, v := range r.Fields {
			m[k] = v
		}
		records[i] = m
	}
	return &DataQueryResult{
		SourceID:   resp.SourceId,
		SourceName: resp.SourceName,
		Total:      int(resp.Total),
		Limit:      int(resp.Limit),
		Offset:     int(resp.Offset),
		Records:    records,
		Via:        resp.Via,
	}
}
