// Package grpcserver 提供 gRPC 服务端实现。
//
// 采用 grpc.UnknownServiceHandler + 原始编解码器模式，
// 无需 protoc 生成桩代码即可编译运行。所有 44 个 RPC 方法
// 通过方法名路由到对应的处理器函数。
//
// 当后续引入 protoc-gen-go 生成桩代码后，可平滑迁移为
// 类型安全的 RegisterPrivacyServiceServer 模式。
package grpcserver

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	_ "google.golang.org/grpc/encoding/gzip" // 注册 gzip 压缩器
	"google.golang.org/grpc/status"

	"github.com/fengzhizi319/PrivShield/engine-go/internal/service"
)

// ──────────────────────────────────────────────
// 原始编解码器（透传 protobuf 字节）
// ──────────────────────────────────────────────

// rawCodec 实现 grpc.Codec 接口，用于透传预序列化的 protobuf 字节。
// 当配合 UnknownServiceHandler 使用时，服务端可直接收发 []byte，
// 避免对未注册消息类型的反序列化错误。
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

func (rawCodec) Name() string { return "proto" }

// ──────────────────────────────────────────────
// gRPC 服务端
// ──────────────────────────────────────────────

// Server gRPC 隐私服务服务端
type Server struct {
	svc      *service.PrivacyService
	grpcSrv  *grpc.Server
	grpcOpts []grpc.ServerOption // 额外 gRPC 选项（如 mTLS 拦截器）
}

// NewServer 创建 gRPC 服务端实例
// 可选传入 grpc.ServerOption（如 mTLS CN 白名单拦截器）
func NewServer(svc *service.PrivacyService, opts ...grpc.ServerOption) *Server {
	return &Server{svc: svc, grpcOpts: opts}
}

// Serve 启动 gRPC 服务（阻塞）
func (s *Server) Serve(lis net.Listener) error {
	// 内置选项：rawCodec + UnknownServiceHandler + 压缩与消息限制
	builtinOpts := []grpc.ServerOption{
		grpc.ForceServerCodec(rawCodec{}),
		grpc.UnknownServiceHandler(s.handleStream),
		grpc.MaxRecvMsgSize(64 * 1024 * 1024), // 64MB 接收上限，防止 OOM
		grpc.MaxSendMsgSize(64 * 1024 * 1024), // 64MB 发送上限
		grpc.MaxConcurrentStreams(250),         // 并发流限制
	}
	// 合并外部传入的选项（如 mTLS 拦截器）
	allOpts := append(builtinOpts, s.grpcOpts...)
	s.grpcSrv = grpc.NewServer(allOpts...)
	slog.Info("gRPC server starting", "addr", lis.Addr())
	return s.grpcSrv.Serve(lis)
}

// GracefulStop 优雅停机
func (s *Server) GracefulStop() {
	if s.grpcSrv != nil {
		s.grpcSrv.GracefulStop()
	}
}

// handleStream 统一流处理入口，按方法名路由
func (s *Server) handleStream(_ interface{}, ss grpc.ServerStream) error {
	method, ok := grpc.MethodFromServerStream(ss)
	if !ok {
		return status.Errorf(codes.Internal, "failed to get method name")
	}

	// 提取短方法名: /privacy.local.PrivacyService/Mask → Mask
	shortName := method
	if idx := strings.LastIndex(method, "/"); idx >= 0 {
		shortName = method[idx+1:]
	}

	start := time.Now()

	// 接收原始请求
	var reqBytes []byte
	if err := ss.RecvMsg(&reqBytes); err != nil {
		return status.Errorf(codes.Internal, "recv: %v", err)
	}

	// 路由到处理器
	respBytes, err := s.route(shortName, reqBytes)

	elapsed := time.Since(start)
	slog.Debug("gRPC call",
		"method", shortName,
		"elapsed_us", elapsed.Microseconds(),
		"err", err,
	)

	if err != nil {
		return err
	}

	return ss.SendMsg(&respBytes)
}

// route 方法路由
func (s *Server) route(method string, reqBytes []byte) ([]byte, error) {
	switch method {
	case "Health":
		return s.handleHealth(reqBytes)
	case "Mask":
		return s.handleMask(reqBytes)
	case "MaskRecord":
		return s.handleMaskRecord(reqBytes)
	case "MaskBatch":
		return s.handleMaskBatch(reqBytes)
	case "Hash":
		return s.handleHash(reqBytes)
	case "DPCount", "DPNoisyCount":
		return s.handleDPNoisyCount(reqBytes)
	case "DPSum", "DPNoisySum":
		return s.handleDPNoisySum(reqBytes)
	case "DPMean", "DPNoisyMean":
		return s.handleDPNoisyMean(reqBytes)
	case "DynClassify":
		return s.handleDynClassify(reqBytes)
	case "ObfuscateQuery":
		return s.handleObfuscateQuery(reqBytes)
	case "KAnonymizeRecord":
		return s.handleKAnonymizeRecord(reqBytes)
	default:
		// 尚未实现的方法返回 JSON 包装的通用响应
		return s.handleGeneric(method, reqBytes)
	}
}

// ──────────────────────────────────────────────
// JSON 辅助（用于 gRPC 字节流内的 JSON 载荷）
// ──────────────────────────────────────────────
// 注意：标准 gRPC 使用 protobuf 编码。此处使用 JSON 作为
// 无桩模式下的载荷格式。生产环境应在引入 protoc-gen-go 后
// 迁移为标准 protobuf 编码。

func parseJSON(data []byte, v interface{}) error {
	return json.Unmarshal(data, v)
}

func toJSON(v interface{}) ([]byte, error) {
	return json.Marshal(v)
}

// ──────────────────────────────────────────────
// 核心 RPC 处理器
// ──────────────────────────────────────────────

func (s *Server) handleHealth(_ []byte) ([]byte, error) {
	resp := map[string]string{
		"status":    "ok",
		"namespace": "default",
		"engine":    "go",
	}
	return toJSON(resp)
}

func (s *Server) handleMask(reqBytes []byte) ([]byte, error) {
	var req struct {
		FieldName string `json:"field_name"`
		Value     string `json:"value"`
		Context   string `json:"context"`
	}
	if err := parseJSON(reqBytes, &req); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "parse: %v", err)
	}

	// 根据字段名推断脱敏类型
	maskType := inferMaskType(req.FieldName)
	result, err := s.svc.MaskField(maskType, req.Value)
	if err != nil {
		// 未知类型直接返回原值
		result = req.Value
	}

	return toJSON(map[string]string{"result": result})
}

func (s *Server) handleMaskRecord(reqBytes []byte) ([]byte, error) {
	var req struct {
		Record  map[string]string `json:"record"`
		Context string            `json:"context"`
	}
	if err := parseJSON(reqBytes, &req); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "parse: %v", err)
	}

	result := s.svc.MaskRecord(req.Record)
	return toJSON(map[string]map[string]string{"result": result})
}

func (s *Server) handleMaskBatch(reqBytes []byte) ([]byte, error) {
	var req struct {
		FieldNames []string          `json:"field_names"`
		Values     []string          `json:"values"`
		Records    []map[string]string `json:"records"`
		Context    string            `json:"context"`
	}
	if err := parseJSON(reqBytes, &req); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "parse: %v", err)
	}

	// 如果有 records 字段则批量脱敏
	if len(req.Records) > 0 {
		results := s.svc.MaskBatch(req.Records)
		return toJSON(map[string]interface{}{"results": results})
	}

	// 否则按 field_names + values 逐字段脱敏
	results := make([]string, len(req.Values))
	for i, v := range req.Values {
		fieldType := "default"
		if i < len(req.FieldNames) {
			fieldType = inferMaskType(req.FieldNames[i])
		}
		r, err := s.svc.MaskField(fieldType, v)
		if err != nil {
			results[i] = v
		} else {
			results[i] = r
		}
	}
	return toJSON(map[string]interface{}{"results": results})
}

func (s *Server) handleHash(reqBytes []byte) ([]byte, error) {
	var req struct {
		Value     string `json:"value"`
		Salt      string `json:"salt"`
		Algorithm string `json:"algorithm"`
	}
	if err := parseJSON(reqBytes, &req); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "parse: %v", err)
	}

	var result string
	if req.Algorithm == "sm3" || req.Algorithm == "hash_sm3" {
		result = s.svc.HashSM3(req.Value, req.Salt)
	} else {
		result = s.svc.HashHMAC(req.Value, req.Salt)
	}
	return toJSON(map[string]string{"result": result})
}

func (s *Server) handleDPNoisyCount(reqBytes []byte) ([]byte, error) {
	var req struct {
		TrueCount float64 `json:"true_count"`
		Epsilon   float64 `json:"epsilon"`
		Mechanism string  `json:"mechanism"`
		Delta     float64 `json:"delta"`
	}
	if err := parseJSON(reqBytes, &req); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "parse: %v", err)
	}

	result, err := s.svc.NoisyCount(context.Background(), int(req.TrueCount), req.Epsilon)
	if err != nil {
		return nil, status.Errorf(codes.ResourceExhausted, "%v", err)
	}
	return toJSON(map[string]float64{"result": result})
}

func (s *Server) handleDPNoisySum(reqBytes []byte) ([]byte, error) {
	var req struct {
		TrueSum     float64 `json:"true_sum"`
		Epsilon     float64 `json:"epsilon"`
		Sensitivity float64 `json:"sensitivity"`
	}
	if err := parseJSON(reqBytes, &req); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "parse: %v", err)
	}

	result, err := s.svc.NoisySum(context.Background(), []float64{req.TrueSum}, req.Epsilon, req.Sensitivity)
	if err != nil {
		return nil, status.Errorf(codes.ResourceExhausted, "%v", err)
	}
	return toJSON(map[string]float64{"result": result})
}

func (s *Server) handleDPNoisyMean(reqBytes []byte) ([]byte, error) {
	var req struct {
		TrueSum   float64 `json:"true_sum"`
		TrueCount float64 `json:"true_count"`
		Epsilon   float64 `json:"epsilon"`
		Delta     float64 `json:"delta"`
		Sensitivity float64 `json:"sensitivity"`
	}
	if err := parseJSON(reqBytes, &req); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "parse: %v", err)
	}

	result, err := s.svc.NoisyMean(context.Background(), []float64{req.TrueSum}, req.Epsilon, req.Delta, req.Sensitivity)
	if err != nil {
		return nil, status.Errorf(codes.ResourceExhausted, "%v", err)
	}
	return toJSON(map[string]float64{"result": result})
}

func (s *Server) handleDynClassify(reqBytes []byte) ([]byte, error) {
	var req struct {
		FieldName  string `json:"field_name"`
		FieldValue string `json:"field_value"`
		Domain     string `json:"domain"`
		Standard   string `json:"standard"`
	}
	if err := parseJSON(reqBytes, &req); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "parse: %v", err)
	}

	result := s.svc.Classify(req.FieldName, req.FieldValue)
	return toJSON(map[string]interface{}{
		"tags": []map[string]interface{}{
			{
				"level":         string(result.Level),
				"category":      result.Category,
				"rule_id":       result.MatchedBy,
				"source_engine": "rule",
				"domain":        req.Domain,
			},
		},
		"max_level":       string(result.Level),
		"audit_timestamp": time.Now().UTC().Format(time.RFC3339),
		"engine_layer":    result.MatchedBy,
	})
}

func (s *Server) handleObfuscateQuery(reqBytes []byte) ([]byte, error) {
	var req struct {
		Query      string   `json:"query"`
		NumDummies int      `json:"num_dummies"`
		Domain     string   `json:"domain"`
		MedicalPool []string `json:"medical_pool"`
		GenericPool []string `json:"generic_pool"`
	}
	if err := parseJSON(reqBytes, &req); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "parse: %v", err)
	}

	queries, _ := s.svc.ObfuscateQuery(req.Query, req.NumDummies, req.Domain)
	return toJSON(map[string]interface{}{"result": queries})
}

func (s *Server) handleKAnonymizeRecord(reqBytes []byte) ([]byte, error) {
	var req struct {
		Record map[string]string `json:"record"`
		QICols []string          `json:"qi_cols"`
		K      int               `json:"k"`
	}
	if err := parseJSON(reqBytes, &req); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "parse: %v", err)
	}

	// K-匿名需要数据集级别操作，单记录直接返回泛化结果
	result := s.svc.MaskRecord(req.Record)
	return toJSON(map[string]map[string]string{"result": result})
}

func (s *Server) handleGeneric(method string, _ []byte) ([]byte, error) {
	slog.Warn("gRPC method not yet implemented", "method", method)
	// 返回空 JSON 响应而非错误，保持向前兼容
	return toJSON(map[string]string{
		"status":  "unimplemented",
		"method":  method,
		"message": "method not yet implemented in Go engine, use Python engine",
	})
}

// ──────────────────────────────────────────────
// 辅助函数
// ──────────────────────────────────────────────

// inferMaskType 根据字段名推断脱敏类型
func inferMaskType(fieldName string) string {
	lower := strings.ToLower(fieldName)
	switch {
	case containsAny(lower, "id_card", "idcard", "cert_no", "identity", "身份证"):
		return "id_card"
	case containsAny(lower, "phone", "mobile", "tel", "手机", "电话"):
		return "phone"
	case containsAny(lower, "bank", "credit_card", "银行卡"):
		return "bank_card"
	case containsAny(lower, "email", "mail", "邮箱"):
		return "email"
	case containsAny(lower, "address", "addr", "地址"):
		return "address"
	case containsAny(lower, "name", "姓名"):
		return "name"
	case containsAny(lower, "officer", "军官"):
		return "officer_id"
	default:
		return "default"
	}
}

func containsAny(s string, substrs ...string) bool {
	for _, sub := range substrs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
