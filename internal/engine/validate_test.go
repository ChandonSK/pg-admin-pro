package engine_test

import (
	"testing"

	"pg-admin-pro/internal/engine"
)

func TestValidateName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid simple", "students", false},
		{"valid with underscore", "edulentra_dadc", false},
		{"empty", "   ", true},
		{"null byte", "bad\x00name", true},
		{"too long", string(make([]byte, 129)), true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := engine.ValidateName(tc.input)
			if (err != nil) != tc.wantErr {
				t.Fatalf("ValidateName(%q) error = %v, wantErr = %v", tc.input, err, tc.wantErr)
			}
		})
	}
}

func TestValidateColumnType(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"integer", "INTEGER", false},
		{"varchar with length", "VARCHAR(255)", false},
		{"numeric precision", "NUMERIC(12, 2)", false},
		{"text array", "TEXT[]", false},
		{"timestamptz", "timestamp with time zone", false},
		{"sql injection semicolon", "TEXT; DROP TABLE users;--", true},
		{"sql injection comment", "INT -- comment", true},
		{"empty", "", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := engine.ValidateColumnType(tc.input)
			if (err != nil) != tc.wantErr {
				t.Fatalf("ValidateColumnType(%q) error = %v, wantErr = %v", tc.input, err, tc.wantErr)
			}
		})
	}
}

func TestValidateDefaultExpr(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"empty", "", false},
		{"now()", "now()", false},
		{"gen_random_uuid()", "gen_random_uuid()", false},
		{"string literal", "'active'", false},
		{"semicolon injection", "1; DROP TABLE users", true},
		{"line comment", "1 -- bad", true},
		{"block comment", "1 /* bad */", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := engine.ValidateDefaultExpr(tc.input)
			if (err != nil) != tc.wantErr {
				t.Fatalf("ValidateDefaultExpr(%q) error = %v, wantErr = %v", tc.input, err, tc.wantErr)
			}
		})
	}
}
