// Package grpcserver 提供类型安全的 gRPC PrivacyService 实现。
//
// 使用 protoc-gen-go 生成的桩代码，实现 proto/privacy.proto 定义的
// 44 个 RPC 方法中的核心方法。未实现的方法通过嵌入 UnimplementedPrivacyServiceServer
// 返回 Unimplemented 状态码。
package grpcserver

import (
	"context"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/fengzhizi319/PrivShield/engine-go/internal/service"
	pb "github.com/fengzhizi319/PrivShield/engine-go/internal/grpcserver/proto"
)

// TypedServer 类型安全的 gRPC 隐私服务端
type TypedServer struct {
	pb.UnimplementedPrivacyServiceServer // 前向兼容
	svc *service.PrivacyService
}

// NewTypedServer 创建类型安全 gRPC 服务端
func NewTypedServer(svc *service.PrivacyService) *TypedServer {
	return &TypedServer{svc: svc}
}

// ──────────────────────────────────────────────
// 核心 RPC 实现
// ──────────────────────────────────────────────

// Health 健康检查
func (s *TypedServer) Health(_ context.Context, _ *pb.HealthRequest) (*pb.HealthResponse, error) {
	return &pb.HealthResponse{
		Status:    "ok",
		Namespace: "default",
	}, nil
}

// Mask 单字段脱敏
func (s *TypedServer) Mask(_ context.Context, req *pb.MaskRequest) (*pb.MaskResponse, error) {
	maskType := inferMaskType(req.GetFieldName())
	result, err := s.svc.MaskField(maskType, req.GetValue())
	if err != nil {
		return &pb.MaskResponse{Result: req.GetValue()}, nil
	}
	return &pb.MaskResponse{Result: result}, nil
}

// MaskRecord 记录级脱敏
func (s *TypedServer) MaskRecord(_ context.Context, req *pb.MaskRecordRequest) (*pb.MaskRecordResponse, error) {
	result := s.svc.MaskRecord(req.GetRecord())
	return &pb.MaskRecordResponse{Result: result}, nil
}

// MaskBatch 批量脱敏
func (s *TypedServer) MaskBatch(_ context.Context, req *pb.MaskBatchRequest) (*pb.MaskBatchResponse, error) {
	// 如果有 field_names + values，逐字段脱敏
	if len(req.GetFieldNames()) > 0 && len(req.GetValues()) > 0 {
		results := make([]string, len(req.GetValues()))
		for i, v := range req.GetValues() {
			fieldType := "default"
			if i < len(req.GetFieldNames()) {
				fieldType = inferMaskType(req.GetFieldNames()[i])
			}
			r, err := s.svc.MaskField(fieldType, v)
			if err != nil {
				results[i] = v
			} else {
				results[i] = r
			}
		}
		return &pb.MaskBatchResponse{Results: results}, nil
	}
	return nil, status.Error(codes.InvalidArgument, "provide field_names+values or use MaskRecord")
}

// Hash HMAC 加盐散列
func (s *TypedServer) Hash(_ context.Context, req *pb.HashRequest) (*pb.HashResponse, error) {
	return &pb.HashResponse{Result: s.svc.HashHMAC(req.GetValue(), req.GetSalt())}, nil
}

// DPNoisyCount 差分隐私噪声计数
func (s *TypedServer) DPNoisyCount(ctx context.Context, req *pb.DPNoisyCountRequest) (*pb.DPResponse, error) {
	result, err := s.svc.NoisyCount(ctx, int(req.GetTrueCount()), req.GetEpsilon())
	if err != nil {
		return nil, status.Error(codes.ResourceExhausted, err.Error())
	}
	return &pb.DPResponse{Result: result}, nil
}

// DPNoisySum 差分隐私噪声求和
func (s *TypedServer) DPNoisySum(ctx context.Context, req *pb.DPNoisySumRequest) (*pb.DPResponse, error) {
	result, err := s.svc.NoisySum(ctx, []float64{req.GetTrueSum()}, req.GetEpsilon(), req.GetSensitivity())
	if err != nil {
		return nil, status.Error(codes.ResourceExhausted, err.Error())
	}
	return &pb.DPResponse{Result: result}, nil
}

// DPNoisyMean 差分隐私噪声均值
func (s *TypedServer) DPNoisyMean(ctx context.Context, req *pb.DPNoisyMeanRequest) (*pb.DPResponse, error) {
	result, err := s.svc.NoisyMean(ctx, []float64{req.GetTrueSum()}, req.GetEpsilon(), req.GetDelta(), req.GetSensitivity())
	if err != nil {
		return nil, status.Error(codes.ResourceExhausted, err.Error())
	}
	return &pb.DPResponse{Result: result}, nil
}

// DPCount / DPSum / DPMean 复用相同逻辑
func (s *TypedServer) DPCount(ctx context.Context, req *pb.DPRequest) (*pb.DPResponse, error) {
	if len(req.GetValues()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "values required")
	}
	result, err := s.svc.NoisyCount(ctx, int(req.GetValues()[0]), req.GetEpsilon())
	if err != nil {
		return nil, status.Error(codes.ResourceExhausted, err.Error())
	}
	return &pb.DPResponse{Result: result}, nil
}

func (s *TypedServer) DPSum(ctx context.Context, req *pb.DPRequest) (*pb.DPResponse, error) {
	sensitivity := 1.0
	if req.GetClipUpper() > 0 {
		sensitivity = req.GetClipUpper() - req.GetClipLower()
	}
	result, err := s.svc.NoisySum(ctx, req.GetValues(), req.GetEpsilon(), sensitivity)
	if err != nil {
		return nil, status.Error(codes.ResourceExhausted, err.Error())
	}
	return &pb.DPResponse{Result: result}, nil
}

func (s *TypedServer) DPMean(ctx context.Context, req *pb.DPRequest) (*pb.DPResponse, error) {
	sensitivity := 1.0
	if req.GetClipUpper() > 0 {
		sensitivity = req.GetClipUpper()
	}
	result, err := s.svc.NoisyMean(ctx, req.GetValues(), req.GetEpsilon(), req.GetDelta(), sensitivity)
	if err != nil {
		return nil, status.Error(codes.ResourceExhausted, err.Error())
	}
	return &pb.DPResponse{Result: result}, nil
}

// KAnonymizeRecord K-匿名单记录
func (s *TypedServer) KAnonymizeRecord(_ context.Context, req *pb.KAnonymizeRequest) (*pb.KAnonymizeResponse, error) {
	result := s.svc.MaskRecord(req.GetRecord())
	return &pb.KAnonymizeResponse{Result: result}, nil
}

// ObfuscateQuery 查询混淆
func (s *TypedServer) ObfuscateQuery(_ context.Context, req *pb.ObfuscateQueryRequest) (*pb.ObfuscateQueryResponse, error) {
	queries, _ := s.svc.ObfuscateQuery(req.GetQuery(), int(req.GetNumDummies()), req.GetDomain())
	return &pb.ObfuscateQueryResponse{Result: queries}, nil
}

// DynClassify 动态分类分级
func (s *TypedServer) DynClassify(_ context.Context, req *pb.DynClassificationRequest) (*pb.DynClassificationResponse, error) {
	result := s.svc.Classify(req.GetFieldName(), req.GetFieldValue())

	tag := &pb.DynSecurityTagProto{
		Level:         string(result.Level),
		Category:      result.Category,
		RuleId:        result.MatchedBy,
		SourceEngine:  "rule",
		Domain:        req.GetDomain(),
		StandardId:    req.GetStandard(),
		MatchTarget:   "field_name",
	}

	return &pb.DynClassificationResponse{
		Tags:           []*pb.DynSecurityTagProto{tag},
		MaxLevel:       string(result.Level),
		AuditTimestamp: time.Now().UTC().Format(time.RFC3339),
		EngineLayer:    result.MatchedBy,
	}, nil
}
