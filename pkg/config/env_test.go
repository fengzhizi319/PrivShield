package config

import (
	"os"
	"testing"
)

func TestEnvString(t *testing.T) {
	os.Setenv("TEST_STR", "hello")
	defer os.Unsetenv("TEST_STR")

	if got := EnvString("TEST_STR", "default"); got != "hello" {
		t.Errorf("EnvString(hello) = %q, want hello", got)
	}
	if got := EnvString("TEST_STR_MISSING", "default"); got != "default" {
		t.Errorf("EnvString(missing) = %q, want default", got)
	}
}

func TestEnvInt(t *testing.T) {
	os.Setenv("TEST_INT", "42")
	defer os.Unsetenv("TEST_INT")

	if got := EnvInt("TEST_INT", 0); got != 42 {
		t.Errorf("EnvInt(42) = %d, want 42", got)
	}
	if got := EnvInt("TEST_INT_MISSING", 99); got != 99 {
		t.Errorf("EnvInt(missing) = %d, want 99", got)
	}

	os.Setenv("TEST_INT_BAD", "not-a-number")
	defer os.Unsetenv("TEST_INT_BAD")
	if got := EnvInt("TEST_INT_BAD", 7); got != 7 {
		t.Errorf("EnvInt(bad) = %d, want 7", got)
	}
}

func TestEnvBool(t *testing.T) {
	tests := []struct {
		value    string
		expected bool
	}{
		{"true", true},
		{"TRUE", true},
		{"1", true},
		{"yes", true},
		{"on", true},
		{"false", false},
		{"0", false},
		{"no", false},
		{"", false},
	}

	for _, tt := range tests {
		os.Setenv("TEST_BOOL", tt.value)
		got := EnvBool("TEST_BOOL", false)
		if got != tt.expected {
			t.Errorf("EnvBool(%q) = %v, want %v", tt.value, got, tt.expected)
		}
	}
	os.Unsetenv("TEST_BOOL")

	// Test default
	if got := EnvBool("TEST_BOOL_MISSING", true); got != true {
		t.Errorf("EnvBool(missing, true) = %v, want true", got)
	}
}

func TestEnvStringSlice(t *testing.T) {
	os.Setenv("TEST_SLICE", "a,b,c")
	defer os.Unsetenv("TEST_SLICE")

	got := EnvStringSlice("TEST_SLICE")
	if len(got) != 3 || got[0] != "a" || got[1] != "b" || got[2] != "c" {
		t.Errorf("EnvStringSlice(a,b,c) = %v, want [a b c]", got)
	}

	// Empty returns nil
	got2 := EnvStringSlice("TEST_SLICE_MISSING")
	if len(got2) != 0 {
		t.Errorf("EnvStringSlice(missing) = %v, want empty", got2)
	}
}
