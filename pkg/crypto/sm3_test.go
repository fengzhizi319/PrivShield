package crypto

import (
	"encoding/hex"
	"testing"
)

func TestSM3_StandardVector1(t *testing.T) {
	// Standard Test Vector 1: "abc"
	// Expected: 66c7f0f462eeedd9d1f2d46bdc10e4e24167c4875cf2f7a2297da02b8f4ba8e0
	input := []byte("abc")
	expected := "66c7f0f462eeedd9d1f2d46bdc10e4e24167c4875cf2f7a2297da02b8f4ba8e0"

	digest := SumSM3(input)
	got := hex.EncodeToString(digest[:])

	if got != expected {
		t.Errorf("SumSM3(%q) = %s, want %s", input, got, expected)
	}
}

func TestSM3_StandardVector2(t *testing.T) {
	// Standard Test Vector 2: 64-byte repeated string
	// Expected: debe9ff92275b8a138604889c18e5a4d6fdb70e5387e5765293dcba39c0c5732
	input := []byte("abcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcd")
	expected := "debe9ff92275b8a138604889c18e5a4d6fdb70e5387e5765293dcba39c0c5732"

	digest := SumSM3(input)
	got := hex.EncodeToString(digest[:])

	if got != expected {
		t.Errorf("SumSM3(vector2) = %s, want %s", got, expected)
	}
}

func TestSM3_HashInterface(t *testing.T) {
	h := NewSM3()
	h.Write([]byte("a"))
	h.Write([]byte("bc"))
	got := hex.EncodeToString(h.Sum(nil))
	expected := "66c7f0f462eeedd9d1f2d46bdc10e4e24167c4875cf2f7a2297da02b8f4ba8e0"

	if got != expected {
		t.Errorf("NewSM3().Write() = %s, want %s", got, expected)
	}
}
