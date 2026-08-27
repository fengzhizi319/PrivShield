// Observability hook for the canonical naming registry.
// canonical 注册表的可观测性钩子（api_rename_design.md §7.2）。
//
// pkg/naming is the single choke point every service passes through when it
// resolves an inbound identifier, so it is also the only place that can count
// alias traffic and normalization failures without each service re-implementing
// the instrumentation ("指标复用 pkg/metrics Collector，避免各服务重复造轮子").
//
// Services register their collector once at startup:
//
//	mc := metrics.NewCollector("service-hub")
//	naming.SetObserver(mc) // *metrics.Collector satisfies naming.Observer
//
// Leaving the observer unset (the default) is a no-op, so unit tests and
// libraries that never export /metrics are unaffected.

package naming

import "sync"

// Observer receives naming-resolution events. *metrics.Collector implements
// this interface directly.
// Observer 接收标识解析事件；*metrics.Collector 天然实现该接口。
type Observer interface {
	// RecordAPIAlias reports that a non-canonical representation was used.
	// target: "datasource_id" | "api_code" | "path"
	RecordAPIAlias(alias, canonical, target string)
	// RecordNormalizeError reports a normalization failure.
	// reason: ReasonUnknown | ReasonEmpty | ReasonReserved
	RecordNormalizeError(reason string)
}

// Metric label value constants / 指标标签取值常量。
const (
	TargetDataSourceID = "datasource_id" // alias 指向 datasource_id
	TargetAPICode      = "api_code"      // 入站值是 api_code
	TargetPath         = "path"          // 入站信号来自废弃端点路径本身

	ReasonUnknown       = "unknown"        // 未命中注册表
	ReasonEmpty         = "empty"          // 空值
	ReasonReserved      = "reserved"       // 命中预留位（写侧 409）
	ReasonFormatInvalid = "format_invalid" // 字面格式不合法
)

var (
	observerMu sync.RWMutex
	observer   Observer
)

// SetObserver registers the naming Observer. Passing nil clears it.
// SetObserver 注册观测器；传 nil 表示清除。
func SetObserver(o Observer) {
	observerMu.Lock()
	defer observerMu.Unlock()
	observer = o
}

// CurrentObserver returns the registered observer (may be nil).
// CurrentObserver 返回当前注册的观测器（可能为 nil）。
func CurrentObserver() Observer {
	observerMu.RLock()
	defer observerMu.RUnlock()
	return observer
}

// recordAlias notifies the observer about an alias representation.
// recordAlias 上报别名使用情况。
func recordAlias(alias, canonical, target string) {
	if o := CurrentObserver(); o != nil {
		o.RecordAPIAlias(alias, canonical, target)
	}
}

// recordNormalizeError notifies the observer about a normalization failure.
// recordNormalizeError 上报归一化失败原因。
func recordNormalizeError(reason string) {
	if o := CurrentObserver(); o != nil {
		o.RecordNormalizeError(reason)
	}
}
