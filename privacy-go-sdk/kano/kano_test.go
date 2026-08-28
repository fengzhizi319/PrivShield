package kano

import (
	"testing"
)

func TestAnonymizeEmpty(t *testing.T) {
	result, err := Anonymize(nil, []string{"age"}, 2)
	if err != nil {
		t.Fatalf("Anonymize(nil) error: %v", err)
	}
	if result == nil {
		t.Fatal("result should not be nil")
	}
}

func TestAnonymizeBasic(t *testing.T) {
	records := []Record{
		{"name": "Alice", "age": "30", "city": "Beijing"},
		{"name": "Bob", "age": "30", "city": "Beijing"},
		{"name": "Charlie", "age": "25", "city": "Shanghai"},
		{"name": "Dave", "age": "25", "city": "Shanghai"},
		{"name": "Eve", "age": "35", "city": "Beijing"},
		{"name": "Frank", "age": "35", "city": "Beijing"},
	}

	result, err := Anonymize(records, []string{"age", "city"}, 2)
	if err != nil {
		t.Fatalf("Anonymize error: %v", err)
	}

	if len(result.Records) != 6 {
		t.Errorf("expected 6 records, got %d", len(result.Records))
	}

	if result.K != 2 {
		t.Errorf("expected k=2, got %d", result.K)
	}

	if result.GroupCount < 1 {
		t.Errorf("expected at least 1 group, got %d", result.GroupCount)
	}
}

func TestAnonymizeSmallDataset(t *testing.T) {
	records := []Record{
		{"name": "Alice", "age": "30"},
	}

	result, err := Anonymize(records, []string{"age"}, 2)
	if err != nil {
		t.Fatalf("Anonymize error: %v", err)
	}

	if len(result.Records) != 1 {
		t.Errorf("expected 1 record, got %d", len(result.Records))
	}
}

func TestIsNumeric(t *testing.T) {
	tests := []struct {
		values   []string
		expected bool
	}{
		{[]string{"1", "2", "3"}, true},
		{[]string{"1.5", "2.7"}, true},
		{[]string{"-1", "2"}, true},
		{[]string{"abc", "def"}, false},
		{[]string{"1", "abc"}, false},
	}

	for _, tt := range tests {
		result := isNumeric(tt.values)
		if result != tt.expected {
			t.Errorf("isNumeric(%v) = %v, want %v", tt.values, result, tt.expected)
		}
	}
}

func TestUniqueValues(t *testing.T) {
	values := []string{"a", "b", "a", "c", "b"}
	result := uniqueValues(values)
	if len(result) != 3 {
		t.Errorf("uniqueValues length = %d, want 3", len(result))
	}
}

func TestCommonPrefix(t *testing.T) {
	tests := []struct {
		a, b     string
		expected string
	}{
		{"Beijing", "Beijing", "Beijing"},
		{"Beijing", "Beijing1", "Beijing"},
		{"abc", "xyz", ""},
		{"上海", "上海", "上海"},
	}

	for _, tt := range tests {
		result := commonPrefix(tt.a, tt.b)
		if result != tt.expected {
			t.Errorf("commonPrefix(%q, %q) = %q, want %q", tt.a, tt.b, result, tt.expected)
		}
	}
}
