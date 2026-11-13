package postgres

import (
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestQuoteLiteral(t *testing.T) {
	got, err := quoteLiteral(`pa'ss\word` + "\nline2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := `E'pa''ss\\word\nline2'`
	if got != want {
		t.Fatalf("quoteLiteral = %q, want %q", got, want)
	}

	if _, err := quoteLiteral("bad\x00pass"); err == nil {
		t.Fatalf("expected error on null byte")
	}
}

func TestNormalizeValueUUID(t *testing.T) {
	u := pgtype.UUID{
		Bytes: [16]byte{0x55, 0x0e, 0x84, 0x00, 0xe2, 0x9b, 0x41, 0xd4, 0xa7, 0x16, 0x44, 0x66, 0x55, 0x44, 0x00, 0x00},
		Valid: true,
	}
	got := normalizeValue(u)
	if got != "550e8400-e29b-41d4-a716-446655440000" {
		t.Fatalf("unexpected uuid string: %v", got)
	}
}
