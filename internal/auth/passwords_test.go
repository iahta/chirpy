package auth

import (
	"testing"
)

func TestHashPassword(t *testing.T) {
	testCases := []struct {
		input string
	}{
		{"password"},
		{"01234"},
		{"passwordobregado"},
	}
	for _, tc := range testCases {
		result, _ := HashPassword(tc.input)
		err := CheckPasswordHash(result, tc.input)
		if err != nil {
			t.Errorf("CheckPasswordHash(%q, %q) does not equal", result, tc.input)
		}
	}
}
