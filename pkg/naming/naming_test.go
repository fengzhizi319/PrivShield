package naming

import (
	"errors"
	"strings"
	"testing"
)

func TestRegistrySelfConsistency(t *testing.T) {
	if len(Registry) == 0 {
		t.Fatal("registry must not be empty")
	}
	if got := len(AliasConflicts()); got > 0 {
		t.Fatalf("registry has conflicting aliases: %v", AliasConflicts())
	}

	seenID := map[string]bool{}
	seenCode := map[string]bool{}
	for _, e := range Registry {
		if e.DataSourceID == "" {
			t.Fatalf("entry seq=%d has empty datasource_id", e.Seq)
		}
		if seenID[e.DataSourceID] {
			t.Fatalf("duplicate datasource_id %q", e.DataSourceID)
		}
		seenID[e.DataSourceID] = true

		if !ValidDataSourceIDFormat(e.DataSourceID) {
			t.Errorf("datasource_id %q violates ^ds_[a-z][a-z0-9_]{1,30}$", e.DataSourceID)
		}
		if e.APICode != "" {
			if seenCode[e.APICode] {
				t.Fatalf("duplicate api_code %q", e.APICode)
			}
			seenCode[e.APICode] = true
			if !ValidAPICodeFormat(e.APICode) {
				t.Errorf("api_code %q violates ^api[1-9]_[a-z][a-z0-9_]{1,30}$", e.APICode)
			}
		}
		if e.Status != StatusActive && e.Status != StatusReserved {
			t.Errorf("%s: unknown status %q", e.DataSourceID, e.Status)
		}
		if e.Status == StatusActive && e.FieldCount <= 0 {
			t.Errorf("%s: active entry must declare a positive field_count", e.DataSourceID)
		}
		if _, ok := e.DisplayName["zh-CN"]; !ok {
			t.Errorf("%s: missing zh-CN display name", e.DataSourceID)
		}
		if _, ok := e.DisplayName["en-US"]; !ok {
			t.Errorf("%s: missing en-US display name", e.DataSourceID)
		}
		// An alias must never collide with another entry's canonical id,
		// otherwise normalization would be order-dependent.
		for _, a := range e.Aliases {
			if other, ok := EntryByDataSourceID(a); ok && other.DataSourceID != e.DataSourceID {
				t.Errorf("%s: alias %q shadows canonical id of %s", e.DataSourceID, a, other.DataSourceID)
			}
		}
	}
}

func TestCanonicalIDsAreActive(t *testing.T) {
	for _, id := range []string{DSYibao, DSKangyang} {
		if err := CheckWritable(id); err != nil {
			t.Errorf("CheckWritable(%q) = %v, want nil", id, err)
		}
	}
}

func TestNormalizeKnownRepresentations(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		// canonical passes through untouched
		{"ds_yibao", DSYibao},
		{"ds_kangyang", DSKangyang},
		// api_code resolves to its datasource
		{API1Yibao, DSYibao},
		{API2Kangyang, DSKangyang},
		// URL slug / file name / Chinese keyword / category
		{"yibao", DSYibao},
		{"Yibao", DSYibao},
		{"yibao.csv", DSYibao},
		{"医保", DSYibao},
		{"medical", DSYibao},
		{"kangyang", DSKangyang},
		{"KANGYANG.CSV", DSKangyang},
		{"康养", DSKangyang},
		{"healthcare", DSKangyang},
		// surrounding whitespace is tolerated
		{"  ds_yibao  ", DSYibao},
		// reserved placeholders are still resolvable (read side)
		{"mock3", DSMock3},
		{"ds_mock4", DSMock4},
	}
	for _, c := range cases {
		got, err := NormalizeDataSourceID(c.in)
		if err != nil {
			t.Errorf("NormalizeDataSourceID(%q) error: %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("NormalizeDataSourceID(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// Unknown values must fail closed: silently defaulting to one datasource is
// the most dangerous naming defect (api_rename_design.md D-11).
func TestNormalizeUnknownFailsClosed(t *testing.T) {
	for _, in := range []string{"", "   ", "shebao", "ds_shebao", "api3_shebao", "ds_custom", "unknown.csv"} {
		got, err := NormalizeDataSourceID(in)
		if err == nil {
			t.Errorf("NormalizeDataSourceID(%q) = %q, want error", in, got)
			continue
		}
		if !IsUnknownDataSource(err) {
			t.Errorf("NormalizeDataSourceID(%q) error %v must wrap ErrUnknownDataSource", in, err)
		}
		if got != "" {
			t.Errorf("NormalizeDataSourceID(%q) returned %q alongside error", in, got)
		}
	}
}

func TestUnknownErrorCarriesAllowedList(t *testing.T) {
	_, err := NormalizeDataSourceID("shebao")
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrUnknownDataSource) {
		t.Fatalf("error must wrap ErrUnknownDataSource, got %v", err)
	}
	msg := err.Error()
	for _, want := range []string{`"shebao"`, DSYibao, DSKangyang} {
		if !strings.Contains(msg, want) {
			t.Errorf("error message %q must mention %q", msg, want)
		}
	}
}

func TestResolveInboundRejectsReserved(t *testing.T) {
	if _, err := ResolveInbound("yibao"); err != nil {
		t.Fatalf("ResolveInbound(yibao) = %v, want nil", err)
	}
	_, err := ResolveInbound("mock3")
	if err == nil {
		t.Fatal("ResolveInbound(mock3) must reject reserved placeholder")
	}
	if !IsReserved(err) {
		t.Errorf("ResolveInbound(mock3) error %v must wrap ErrReservedDataSource", err)
	}
	if IsUnknownDataSource(err) {
		t.Errorf("reserved error must not be reported as unknown: %v", err)
	}
}

func TestCheckWritableRejectsUnregistered(t *testing.T) {
	if err := CheckWritable("yibao"); err == nil {
		t.Error("CheckWritable must require canonical form, got nil for slug \"yibao\"")
	}
	if err := CheckWritable("ds_nope"); err == nil || !IsUnknownDataSource(err) {
		t.Errorf("CheckWritable(ds_nope) = %v, want ErrUnknownDataSource", err)
	}
}

func TestAPICodeDataSourceBidirectionalMapping(t *testing.T) {
	if got := APICodeForDataSource(DSYibao); got != API1Yibao {
		t.Errorf("APICodeForDataSource(ds_yibao) = %q, want %q", got, API1Yibao)
	}
	if got := APICodeForDataSource(DSMock3); got != "" {
		t.Errorf("reserved placeholder must have no api_code, got %q", got)
	}
	for _, e := range Registry {
		if e.APICode == "" {
			continue
		}
		id, ok := DataSourceForAPICode(e.APICode)
		if !ok || id != e.DataSourceID {
			t.Errorf("DataSourceForAPICode(%q) = %q/%v, want %q", e.APICode, id, ok, e.DataSourceID)
		}
	}
}

func TestActiveEntriesExcludeReserved(t *testing.T) {
	active := ActiveDataSourceIDs()
	if len(active) == 0 {
		t.Fatal("expected at least one active datasource")
	}
	for _, id := range active {
		e, ok := EntryByDataSourceID(id)
		if !ok || e.Status != StatusActive {
			t.Errorf("ActiveDataSourceIDs leaked non-active id %q", id)
		}
	}
	for _, id := range AllDataSourceIDs() {
		e, _ := EntryByDataSourceID(id)
		if e.Status == StatusReserved && strings.Contains(strings.Join(active, ","), id) {
			t.Errorf("reserved id %q leaked into active list", id)
		}
	}
	if len(Entries()) != len(Registry) {
		t.Errorf("Entries() must return all %d rows, got %d", len(Registry), len(Entries()))
	}
}

func TestFormatValidators(t *testing.T) {
	valid := []string{"ds_yibao", "ds_kangyang", "ds_mock3", "ds_a1"}
	invalid := []string{"yibao", "ds_", "ds_Yibao", "ds-1", "d_yibao", "ds_"}
	for _, s := range valid {
		if !ValidDataSourceIDFormat(s) {
			t.Errorf("ValidDataSourceIDFormat(%q) = false", s)
		}
	}
	for _, s := range invalid {
		if ValidDataSourceIDFormat(s) {
			t.Errorf("ValidDataSourceIDFormat(%q) = true", s)
		}
	}
	if !ValidAPICodeFormat(API2Kangyang) {
		t.Error("api2_kangyang must be a valid api_code")
	}
	for _, s := range []string{"API1", "api_1", "api0_yibao", "yibao"} {
		if ValidAPICodeFormat(s) {
			t.Errorf("ValidAPICodeFormat(%q) = true", s)
		}
	}
}
