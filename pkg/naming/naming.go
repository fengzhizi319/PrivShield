// Package naming is the single source of truth for cross-service business
// identifiers: the canonical `api_code` / `datasource_id` registry and the
// inbound alias normalization rules.
//
// Package naming 是跨服务业务标识的唯一事实源：canonical `api_code` /
// `datasource_id` 注册表，以及入站别名的归一化规则。
//
// 设计约束（见 console/app-lz/docs/api_rename_design.md §5、§6）：
//  1. 一个数据源只有一个 datasource_id；slug（yibao）、文件名（yibao.csv）、
//     中文名（医保）、api_code（api1_yibao）都只是它的表现形式，只允许在
//     服务边界被归一化一次，内部各层只传 canonical 值。
//  2. 归一化未知值必须 fail-closed（写侧拒绝），禁止静默回落到默认数据源。
//  3. 业务模块不得再出现裸数据源字面量，一律引用本包常量或查注册表。
package naming

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// canonical api_code —— 业务 API 稳定标识
const (
	API1Yibao    = "api1_yibao"
	API2Kangyang = "api2_kangyang"
)

// canonical datasource_id —— 数据源实体标识
const (
	DSYibao    = "ds_yibao"
	DSKangyang = "ds_kangyang"
	DSMock3    = "ds_mock3" // 预留政务数据源 3
	DSMock4    = "ds_mock4" // 预留企业/金融数据源 4
)

// Registry entry status values.
// 注册表条目状态。
const (
	StatusActive   = "active"   // 已实现，可派发
	StatusReserved = "reserved" // 占位，写侧必须拒绝（409）
)

// ID literal formats (api_rename_design.md §1.3).
// ID 字面格式校验正则。
var (
	datasourceIDRe = regexp.MustCompile(`^ds_[a-z][a-z0-9_]{1,30}$`)
	apiCodeRe      = regexp.MustCompile(`^api[1-9]_[a-z][a-z0-9_]{1,30}$`)
)

// ErrUnknownDataSource is returned when an inbound value cannot be mapped to a
// canonical datasource_id. Callers should translate it into
// 400 INVALID_DATASOURCE_ID (REST) or codes.InvalidArgument (gRPC).
// ErrUnknownDataSource 表示入站值无法映射到 canonical datasource_id。
var ErrUnknownDataSource = errors.New("unknown datasource id")

// ErrReservedDataSource is returned for a registered but not-yet-implemented
// entry; callers should translate it into 409 RESERVED_DATASOURCE.
// ErrReservedDataSource 表示条目已登记但未实现，写侧需拒绝。
var ErrReservedDataSource = errors.New("reserved datasource")

// Entry is one row of the canonical registry.
// Entry 是 canonical 注册表的一行。
type Entry struct {
	APICode      string            // "api1_yibao"；预留位为空
	DataSourceID string            // canonical 数据源 ID
	Seq          int               // 展示序号（仅用于 UI，不参与语义）
	DisplayName  map[string]string // 语言标签 → 展示名（"zh-CN" / "en-US"）
	Category     string            // "medical" | "healthcare" | "reserved"
	FileName     string            // 源数据文件名（仅展示/排查用途）
	FieldCount   int               // schema 字段数
	Aliases      []string          // 入站可接受的别名（slug/文件名/中文名/类别）
	Status       string            // StatusActive | StatusReserved
}

// Registry is the authoritative list of business data sources.
// Registry 是业务数据源的权威清单，顺序即展示顺序。
var Registry = []Entry{
	{
		APICode:      API1Yibao,
		DataSourceID: DSYibao,
		Seq:          1,
		DisplayName:  map[string]string{"zh-CN": "医保结算数据接口", "en-US": "Medical Insurance Settlement API"},
		Category:     "medical",
		FileName:     "yibao.csv",
		FieldCount:   18,
		Aliases:      []string{"yibao", "yibao.csv", "medical.csv", "医保", "医保数据", "医保数据库", "医保结算", "medical", "medical_insurance"},
		Status:       StatusActive,
	},
	{
		APICode:      API2Kangyang,
		DataSourceID: DSKangyang,
		Seq:          2,
		DisplayName:  map[string]string{"zh-CN": "康养健康档案接口", "en-US": "Elderly-Care Health Record API"},
		Category:     "healthcare",
		FileName:     "kangyang.csv",
		FieldCount:   27,
		Aliases:      []string{"kangyang", "kangyang.csv", "healthcare.csv", "康养", "康养数据", "康养数据库", "康养体检", "healthcare", "elderly_care"},
		Status:       StatusActive,
	},
	{
		DataSourceID: DSMock3,
		Seq:          3,
		DisplayName:  map[string]string{"zh-CN": "预留政务数据源 3", "en-US": "Reserved Municipal Dataset 3"},
		Category:     "reserved",
		FileName:     "mock3.csv",
		Aliases:      []string{"mock3", "mock3.csv", "政务", "政务数据", "政务数据源"},
		Status:       StatusReserved,
	},
	{
		DataSourceID: DSMock4,
		Seq:          4,
		DisplayName:  map[string]string{"zh-CN": "预留企业/金融数据源 4", "en-US": "Reserved Enterprise Dataset 4"},
		Category:     "reserved",
		FileName:     "mock4.csv",
		Aliases:      []string{"mock4", "mock4.csv", "企业", "金融", "企业数据", "金融数据"},
		Status:       StatusReserved,
	},
}

// Lookup tables built once at init: canonical id / api_code / alias → Entry.
// init 时构建一次索引：canonical id / api_code / 别名 → Entry。
var (
	byDataSourceID map[string]*Entry
	byAPICode      map[string]*Entry
	aliasIndex     map[string]*Entry
	aliasConflicts []string
)

func init() {
	byDataSourceID = make(map[string]*Entry, len(Registry))
	byAPICode = make(map[string]*Entry, len(Registry))
	aliasIndex = make(map[string]*Entry, len(Registry)*4)

	for i := range Registry {
		e := &Registry[i]
		byDataSourceID[e.DataSourceID] = e
		if e.APICode != "" {
			byAPICode[e.APICode] = e
		}
		// The canonical id itself and its api_code are also valid inbound
		// aliases, but they are resolved with higher priority below.
		for _, a := range e.Aliases {
			if prev, ok := aliasIndex[a]; ok && prev.DataSourceID != e.DataSourceID {
				aliasConflicts = append(aliasConflicts, fmt.Sprintf("%q→%s|%s", a, prev.DataSourceID, e.DataSourceID))
			}
			aliasIndex[a] = e
		}
	}
}

// AliasConflicts returns aliases registered by more than one entry. It must
// always be empty; it is exposed so unit tests can fail loudly on registry
// pollution (api_rename_design.md §6.1 AMBIGUOUS_SOURCE defence).
// AliasConflicts 返回被多个条目占用的别名，正常必须为空。
func AliasConflicts() []string { return aliasConflicts }

// EntryByDataSourceID returns the entry whose canonical id equals id (exact match).
// EntryByDataSourceID 按 canonical ID 精确查找条目。
func EntryByDataSourceID(id string) (Entry, bool) {
	if e, ok := byDataSourceID[id]; ok {
		return *e, true
	}
	return Entry{}, false
}

// EntryByAPICode returns the entry for a canonical api_code (exact match).
// EntryByAPICode 按 canonical api_code 精确查找条目。
func EntryByAPICode(code string) (Entry, bool) {
	if e, ok := byAPICode[code]; ok {
		return *e, true
	}
	return Entry{}, false
}

// Entries returns a copy of the full registry in display order.
// Entries 按展示顺序返回完整注册表副本。
func Entries() []Entry {
	out := make([]Entry, len(Registry))
	copy(out, Registry)
	return out
}

// ActiveEntries returns only implemented (status=active) entries.
// ActiveEntries 只返回已实现（status=active）的条目，供 UI 选项使用。
func ActiveEntries() []Entry {
	out := make([]Entry, 0, len(Registry))
	for _, e := range Registry {
		if e.Status == StatusActive {
			out = append(out, e)
		}
	}
	return out
}

// ActiveDataSourceIDs returns canonical ids that may be dispatched.
// ActiveDataSourceIDs 返回允许派发的 canonical 数据源 ID。
func ActiveDataSourceIDs() []string {
	out := make([]string, 0, len(Registry))
	for _, e := range Registry {
		if e.Status == StatusActive {
			out = append(out, e.DataSourceID)
		}
	}
	return out
}

// AllDataSourceIDs returns every registered canonical id, reserved included.
// AllDataSourceIDs 返回全部已登记 canonical ID（含预留位）。
func AllDataSourceIDs() []string {
	out := make([]string, 0, len(Registry))
	for _, e := range Registry {
		out = append(out, e.DataSourceID)
	}
	return out
}

// NormalizeDataSourceID maps any accepted inbound representation (canonical id,
// api_code, URL slug, source file name, Chinese display keyword) to the
// canonical datasource_id.
//
// 归一化优先级固定为：canonical id > api_code > alias，未知/空值返回包装
// ErrUnknownDataSource 的错误，调用方必须 fail-closed（禁止回落默认源）。
func NormalizeDataSourceID(raw string) (string, error) {
	e, err := Normalize(raw)
	if err != nil {
		return "", err
	}
	return e.DataSourceID, nil
}

// Normalize is the Entry-returning form of NormalizeDataSourceID.
// Normalize 返回条目本体，便于调用方同时取到 api_code / 展示名。
func Normalize(raw string) (*Entry, error) {
	v := strings.TrimSpace(raw)
	if v == "" {
		return nil, unknownError(raw)
	}
	if e, ok := byDataSourceID[v]; ok {
		return e, nil
	}
	if e, ok := byAPICode[v]; ok {
		return e, nil
	}
	// ASCII aliases are case-insensitive; non-ASCII aliases match exactly.
	lowered := strings.ToLower(v)
	if e, ok := aliasIndex[lowered]; ok {
		return e, nil
	}
	if e, ok := aliasIndex[v]; ok {
		return e, nil
	}
	return nil, unknownError(raw)
}

// unknownError builds an error that wraps ErrUnknownDataSource and carries the
// received value plus the allowed canonical ids, so handlers can render the
// {"code":"INVALID_DATASOURCE_ID","details":{...}} body without extra lookup.
func unknownError(received string) error {
	return fmt.Errorf("%w: %q (allowed: %s)", ErrUnknownDataSource, received,
		strings.Join(ActiveDataSourceIDs(), ", "))
}

// IsUnknownDataSource reports whether err came from an unmappable inbound id.
// IsUnknownDataSource 判断错误是否为「未知数据源标识」。
func IsUnknownDataSource(err error) bool {
	return errors.Is(err, ErrUnknownDataSource)
}

// IsReserved reports whether err was caused by a reserved (not implemented) entry.
// IsReserved 判断错误是否由「已登记未实现」条目引起。
func IsReserved(err error) bool {
	return errors.Is(err, ErrReservedDataSource)
}

// CheckWritable validates an already-canonical datasource_id for write-side use:
// it must be registered and must not be reserved.
//
// CheckWritable 校验写侧使用的 canonical datasource_id：必须已登记且非预留位。
func CheckWritable(datasourceID string) error {
	e, ok := EntryByDataSourceID(datasourceID)
	if !ok {
		return unknownError(datasourceID)
	}
	if e.Status != StatusActive {
		return fmt.Errorf("%w: %s", ErrReservedDataSource, datasourceID)
	}
	return nil
}

// ResolveInbound normalizes any accepted representation and enforces the
// write-side rules in one call. It is the single entry point handlers should
// use before persisting or dispatching a task.
//
// ResolveInbound 一次性完成「归一化 + 写侧校验」，是 handler 落库/派发前的唯一入口。
func ResolveInbound(raw string) (string, error) {
	e, err := Normalize(raw)
	if err != nil {
		return "", err
	}
	if e.Status != StatusActive {
		return "", fmt.Errorf("%w: %s", ErrReservedDataSource, e.DataSourceID)
	}
	return e.DataSourceID, nil
}

// ValidDataSourceIDFormat reports whether s matches ^ds_[a-z][a-z0-9_]{1,30}$.
// ValidDataSourceIDFormat 判断字符串是否符合 datasource_id 字面格式。
func ValidDataSourceIDFormat(s string) bool { return datasourceIDRe.MatchString(s) }

// ValidAPICodeFormat reports whether s matches ^api[1-9]_[a-z][a-z0-9_]{1,30}$.
// ValidAPICodeFormat 判断字符串是否符合 api_code 字面格式。
func ValidAPICodeFormat(s string) bool { return apiCodeRe.MatchString(s) }

// APICodeForDataSource returns the canonical api_code of a datasource_id, or ""
// when the entry has no business API code bound yet (reserved placeholders).
// APICodeForDataSource 返回数据源对应的 api_code，预留位返回空串。
func APICodeForDataSource(datasourceID string) string {
	if e, ok := byDataSourceID[datasourceID]; ok {
		return e.APICode
	}
	return ""
}

// DataSourceForAPICode returns the canonical datasource_id of an api_code.
// DataSourceForAPICode 返回 api_code 对应的 canonical 数据源 ID。
func DataSourceForAPICode(apiCode string) (string, bool) {
	if e, ok := byAPICode[apiCode]; ok {
		return e.DataSourceID, true
	}
	return "", false
}

// APICodes returns all registered api_codes in display order.
// APICodes 按展示顺序返回全部已登记 api_code。
func APICodes() []string {
	out := make([]string, 0, len(Registry))
	for _, e := range Registry {
		if e.APICode != "" {
			out = append(out, e.APICode)
		}
	}
	return out
}
