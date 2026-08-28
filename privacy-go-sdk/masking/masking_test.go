package masking

import (
	"testing"
)

func TestMaskIdCard(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "standard 18-digit ID card",
			input:    "110101199001011234",
			expected: "110101********1234",
		},
		{
			name:     "ID card ending with X",
			input:    "11010119900101123X",
			expected: "110101********123X",
		},
		{
			name:     "non-standard length",
			input:    "12345678901234567890",
			expected: "1234************7890",
		},
		{
			name:     "short input",
			input:    "12345",
			expected: "*****",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MaskIdCard(tt.input)
			if result != tt.expected {
				t.Errorf("MaskIdCard(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestMaskPhone(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "standard mobile number",
			input:    "13812345678",
			expected: "138****5678",
		},
		{
			name:     "with +86 prefix",
			input:    "+8613812345678",
			expected: "+86 138****5678",
		},
		{
			name:     "short input",
			input:    "12345",
			expected: "*****",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MaskPhone(tt.input)
			if result != tt.expected {
				t.Errorf("MaskPhone(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestMaskBankCard(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "standard bank card",
			input:    "6222021234567890123",
			expected: "622202*********0123",
		},
		{
			name:     "short input",
			input:    "12345",
			expected: "*****",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MaskBankCard(tt.input)
			if result != tt.expected {
				t.Errorf("MaskBankCard(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestMaskChineseName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "two-character name",
			input:    "张三",
			expected: "张*",
		},
		{
			name:     "three-character name",
			input:    "张三丰",
			expected: "张*丰",
		},
		{
			name:     "four-character name",
			input:    "欧阳三丰",
			expected: "欧**丰",
		},
		{
			name:     "single character",
			input:    "张",
			expected: "*",
		},
		{
			name:     "empty input",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MaskChineseName(tt.input)
			if result != tt.expected {
				t.Errorf("MaskChineseName(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestMaskEmail(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "standard email",
			input:    "test@example.com",
			expected: "te**@example.com",
		},
		{
			name:     "short local part",
			input:    "ab@example.com",
			expected: "ab@example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MaskEmail(tt.input)
			if result != tt.expected {
				t.Errorf("MaskEmail(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestMaskAddress(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "long address",
			input:    "北京市朝阳区建国路88号",
			expected: "北京市朝阳区******",
		},
		{
			name:     "short address",
			input:    "北京市",
			expected: "***",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MaskAddress(tt.input)
			if result != tt.expected {
				t.Errorf("MaskAddress(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestHashHMAC(t *testing.T) {
	result1 := HashHMAC("test", "salt")
	result2 := HashHMAC("test", "salt")
	if result1 != result2 {
		t.Errorf("HashHMAC should be deterministic")
	}

	result3 := HashHMAC("test", "different_salt")
	if result1 == result3 {
		t.Errorf("HashHMAC with different salt should produce different result")
	}
}
