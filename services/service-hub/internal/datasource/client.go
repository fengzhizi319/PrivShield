// Package datasource provides a client for communicating with the datasource-mgr module.
// Package datasource 提供与模拟数据源服务 (datasource-mgr) 通信的客户端，支持 HTTP REST 与 gRPC (mTLS) 双协议。
//
// 架构设计：
// 1. 双协议支持：提供基于 net/http 的 HTTPS REST 客户端与基于 grpc-go 的高性能 gRPC 客户端；
// 2. 线程安全与延迟连接：gRPC 连接采用 sync.RWMutex 读写锁进行并发安全保护与按需懒加载初始化；
// 3. 生产级 mTLS 支持：当启用 TLS 时，自动加载客户端证书/私钥与受信任 CA，建立端到端 TLS 1.3 加密通道；
// 4. 数据协议适配转换：提供 protoToQueryResult 辅助函数，无缝将 gRPC Protobuf 结构转换为通用的 DataQueryResult 实体。
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
	"google.golang.org/grpc/keepalive"

	dspb "github.com/fengzhizi319/PrivShield/services/datasource-mgr/proto"
	naming "github.com/fengzhizi319/PrivShield/pkg/naming"
	"github.com/fengzhizi319/PrivShield/services/service-hub/internal/config"
)

// Client handles HTTP/REST and gRPC communication with datasource-mgr.
// Client 结构体负责与 datasource-mgr 微服务进行双协议通信，管理 HTTP 传输层与 gRPC 连接生命周期。
type Client struct {
	cfg        *config.Config // 全局运行配置引用
	baseURL    string         // datasource-mgr HTTP REST 基础 URL（如 "http://127.0.0.1:8083"）
	grpcAddr   string         // datasource-mgr gRPC 监听网络地址（如 "127.0.0.1:50053"）
	httpClient *http.Client   // 配置了超时与可选 mTLS 的 HTTP 客户端

	mu         sync.RWMutex                         // 保护 gRPC 连接与客户端实例的读写互斥锁
	grpcConn   *grpc.ClientConn                     // gRPC 底层长连接实例
	grpcClient dspb.DataSourceManagerServiceClient  // gRPC 生成桩客户端
}

// New creates a new Client instance with optional HTTPS mTLS support.
// New 构造函数根据传入的配置初始化数据源客户端。
// 执行步骤：
// 1. 构建默认超时为 10 秒的标准 http.Client；
// 2. 若配置了 TLSEnabled 并提供了证书和私钥，则构造 TLS 1.3 配置与 RootCAs 证书池，注入 http.Transport；
// 3. 解析并格式化 HTTP 基地址与 gRPC 目标地址，返回 Client 实例。
func New(cfg *config.Config) *Client {
	httpClient := &http.Client{
		Timeout: 10 * time.Second,
	}

	// 配置 HTTPS 客户端证书双向认证（mTLS）
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
// Close 方法安全关闭当前持有的 gRPC 底层连接，释放网络句柄资源。
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
// Health 通过 HTTP GET /api/health 探测 datasource-mgr 的健康状态。
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
// DataQueryResult 结构体表示从 datasource-mgr 查询抽样获取的标准数据集对象。
type DataQueryResult struct {
	DatasourceID string           `json:"datasource_id"` // canonical 数据源标识（如 "ds_yibao"）
	SourceID     string           `json:"source_id"`     // DEPRECATED 历史字段，兼容双写
	SourceName   string           `json:"source_name"`   // 数据源名称（如 "医保结算高敏数据"）
	Total        int              `json:"total"`         // 数据集总记录条数
	Limit        int              `json:"limit"`         // 分页限制每页大小
	Offset       int              `json:"offset"`        // 分页偏移游标
	Records      []map[string]any `json:"records"`       // 结构化样本数据行切片
	Via          string           `json:"via"`           // 模块来源标识
}

// FetchData requests records from datasource-mgr using the canonical path:
// GET /api/datasources/{id}/records?limit=&offset=
func (c *Client) FetchData(ctx context.Context, datasourceID string, limit, offset int) (*DataQueryResult, error) {
	normID, err := naming.NormalizeDataSourceID(datasourceID)
	if err != nil {
		normID = datasourceID
	}
	res, err := c.fetchEndpoint(ctx, fmt.Sprintf("/api/datasources/%s/records", url.PathEscape(normID)), limit, offset)
	if err != nil {
		return nil, err
	}
	if res.DatasourceID == "" {
		res.DatasourceID = normID
	}
	if res.SourceID == "" {
		res.SourceID = normID
	}
	return res, nil
}

// FetchYibaoData requests mock yibao data (API 1) via HTTP REST.
// FetchYibaoData 发送 GET /api/v1/yibao 请求，获取医保就医结算模拟高敏数据。
func (c *Client) FetchYibaoData(ctx context.Context, limit, offset int) (*DataQueryResult, error) {
	return c.fetchEndpoint(ctx, "/api/v1/yibao", limit, offset)
}

// FetchKangyangData requests mock kangyang data (API 2) via HTTP REST.
// FetchKangyangData 发送 GET /api/v1/kangyang 请求，获取康养体检与慢病档案模拟数据。
func (c *Client) FetchKangyangData(ctx context.Context, limit, offset int) (*DataQueryResult, error) {
	return c.fetchEndpoint(ctx, "/api/v1/kangyang", limit, offset)
}

// FetchMockData3 requests mock data 3 (API 3) via HTTP REST.
// FetchMockData3 发送 GET /api/v1/mock3 请求，获取预留政务数据源 3 模拟数据。
func (c *Client) FetchMockData3(ctx context.Context, limit, offset int) (*DataQueryResult, error) {
	return c.fetchEndpoint(ctx, "/api/v1/mock3", limit, offset)
}

// FetchMockData4 requests mock data 4 (API 4) via HTTP REST.
// FetchMockData4 发送 GET /api/v1/mock4 请求，获取预留企业/金融数据源 4 模拟数据。
func (c *Client) FetchMockData4(ctx context.Context, limit, offset int) (*DataQueryResult, error) {
	return c.fetchEndpoint(ctx, "/api/v1/mock4", limit, offset)
}

// FetchDataBySource dispatches to FetchData via canonical /api/datasources/{id}/records.
func (c *Client) FetchDataBySource(ctx context.Context, sourceID string, limit, offset int) (*DataQueryResult, error) {
	return c.FetchData(ctx, sourceID, limit, offset)
}

// ListDataSources fetches the list of mock datasources via HTTP REST.
// ListDataSources 发起 GET /api/datasources 请求，获取全部已注册数据源的元数据列表。
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
// GetDataSource 发起 GET /api/datasources/:id 请求，获取指定数据源的详细属性。
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
// TestConnection 发起 POST /api/datasources/:id/test 请求，测试与指定数据源的物理连通性。
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

// fetchEndpoint executes an HTTP GET request with limit and offset query parameters.
// fetchEndpoint 内部通用辅助方法：负责解析 URL、附加分页 Query 参数、执行请求并反序列化为 DataQueryResult。
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

// getGRPCClient initializes (if necessary) and returns the cached gRPC service client in a thread-safe manner.
// getGRPCClient 方法以双重检查锁（DCL）线程安全地获取或初始化 gRPC 客户端连接：
// 1. 先加读锁检查 grpcClient 是否已建立，若存在直接复用；
// 2. 加写锁二次检查；
// 3. 根据配置构造 TLS 1.3 凭证（加载 CA 证书与客户端证书）或明文凭证；
// 4. 发起 grpc.DialContext 建立长连接并实例化 DataSourceManagerServiceClient。
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

	// Use a bounded timeout for the dial operation to prevent indefinite blocking
	// when the upstream gRPC service is unreachable.
	// 为连接操作设置有限超时，防止上游 gRPC 服务不可达时无限阻塞。
	dialCtx, dialCancel := context.WithTimeout(ctx, 10*time.Second)
	defer dialCancel()

	conn, err := grpc.DialContext(dialCtx, c.grpcAddr, dialOpt,
		grpc.WithBlock(),
		// Client-side keepalive: detect dead connections and maintain liveness.
		// 客户端 keepalive：检测死连接并维持链路活跃。
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                10 * time.Second,
			Timeout:             5 * time.Second,
			PermitWithoutStream: true,
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("dial datasource-mgr gRPC at %s: %w", c.grpcAddr, err)
	}

	c.grpcConn = conn
	c.grpcClient = dspb.NewDataSourceManagerServiceClient(conn)
	return c.grpcClient, nil
}

// HealthGRPC checks datasource-mgr connectivity via gRPC.
// HealthGRPC 通过 gRPC 调用 Health RPC 方法检测服务健康状态。
func (c *Client) HealthGRPC(ctx context.Context) (*dspb.HealthResponse, error) {
	client, err := c.getGRPCClient(ctx)
	if err != nil {
		return nil, err
	}
	return client.Health(ctx, &dspb.HealthRequest{})
}

// FetchYibaoDataGRPC requests mock yibao data (API 1) via gRPC.
// FetchYibaoDataGRPC 通过 gRPC 调用 GetYibaoData 获取医保结算数据。
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
// FetchKangyangDataGRPC 通过 gRPC 调用 GetKangyangData 获取康养慢病数据。
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
// FetchMockData3GRPC 通过 gRPC 调用 GetMockData3 获取预留政务数据。
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
// FetchMockData4GRPC 通过 gRPC 调用 GetMockData4 获取预留金融数据。
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
// FetchDataBySourceGRPC 通过 gRPC 调用 GetDataBySource 根据源标识动态查询抽样数据。
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
// ListMockSourcesGRPC 通过 gRPC 调用 ListMockSources 获取所有模拟数据源列表。
func (c *Client) ListMockSourcesGRPC(ctx context.Context) (*dspb.ListMockSourcesResponse, error) {
	client, err := c.getGRPCClient(ctx)
	if err != nil {
		return nil, err
	}
	return client.ListMockSources(ctx, &dspb.ListMockSourcesRequest{})
}

// GetDataSourceGRPC gets datasource details via gRPC.
// GetDataSourceGRPC 通过 gRPC 调用 GetDataSource 获取单数据源详情。
func (c *Client) GetDataSourceGRPC(ctx context.Context, id string) (*dspb.DataSourceProto, error) {
	client, err := c.getGRPCClient(ctx)
	if err != nil {
		return nil, err
	}
	return client.GetDataSource(ctx, &dspb.GetDataSourceRequest{Id: id})
}

// TestConnectionGRPC tests connection via gRPC.
// TestConnectionGRPC 通过 gRPC 调用 TestConnection 触发连通性探针。
func (c *Client) TestConnectionGRPC(ctx context.Context, id string) (*dspb.TestConnectionResponse, error) {
	client, err := c.getGRPCClient(ctx)
	if err != nil {
		return nil, err
	}
	return client.TestConnection(ctx, &dspb.TestConnectionRequest{Id: id})
}

// protoToQueryResult converts a Protobuf DataQueryResponse to a standard DataQueryResult domain model.
// protoToQueryResult 将 gRPC Protobuf DataQueryResponse 对象转换为 Go 业务层标准实体 DataQueryResult。
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
	canonID, _ := naming.NormalizeDataSourceID(resp.SourceId)
	if canonID == "" {
		canonID = resp.SourceId
	}
	return &DataQueryResult{
		DatasourceID: canonID,
		SourceID:     canonID,
		SourceName:   resp.SourceName,
		Total:        int(resp.Total),
		Limit:        int(resp.Limit),
		Offset:       int(resp.Offset),
		Records:      records,
		Via:          resp.Via,
	}
}
