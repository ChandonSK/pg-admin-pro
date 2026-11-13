package postgres

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"net/netip"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// quoteIdent safely quotes one or more PostgreSQL identifiers.
func quoteIdent(parts ...string) string {
	return pgx.Identifier(parts).Sanitize()
}

// quoteLiteral safely escapes a PostgreSQL string literal using E'...' syntax.
func quoteLiteral(s string) (string, error) {
	if strings.ContainsRune(s, '\x00') {
		return "", fmt.Errorf("null byte not permitted in SQL literal")
	}
	escaped := strings.ReplaceAll(s, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `'`, `''`)
	escaped = strings.ReplaceAll(escaped, "\n", `\n`)
	escaped = strings.ReplaceAll(escaped, "\r", `\r`)
	return "E'" + escaped + "'", nil
}

// validateName ensures identifiers like database/user names contain no null bytes and are non-empty.
func validateName(name string) (string, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return "", fmt.Errorf("name cannot be empty")
	}
	if len(trimmed) > 128 {
		return "", fmt.Errorf("name exceeds maximum length (128)")
	}
	if strings.ContainsRune(trimmed, '\x00') {
		return "", fmt.Errorf("name contains invalid null byte")
	}
	return trimmed, nil
}

// normalizeValue converts pgx scanned values into clean JSON-friendly Go values.
func normalizeValue(val any) any {
	if val == nil {
		return nil
	}
	switch v := val.(type) {
	case [16]byte:
		return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", v[0:4], v[4:6], v[6:8], v[8:10], v[10:16])
	case pgtype.UUID:
		if !v.Valid {
			return nil
		}
		b := v.Bytes
		return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
	case pgtype.Numeric:
		if !v.Valid {
			return nil
		}
		val, err := v.Value()
		if err == nil {
			return val
		}
		return fmt.Sprintf("%v", v)
	case pgtype.Interval:
		if !v.Valid {
			return nil
		}
		return fmt.Sprintf("%d mons %d days %d us", v.Months, v.Days, v.Microseconds)
	case time.Time:
		return v.Format(time.RFC3339Nano)
	case netip.Prefix:
		return v.String()
	case netip.Addr:
		return v.String()
	case []byte:
		return `\x` + hex.EncodeToString(v)
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return fmt.Sprintf("%v", v)
		}
		return v
	case float32:
		if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
			return fmt.Sprintf("%v", v)
		}
		return v
	case fmt.Stringer:
		return v.String()
	default:
		return v
	}
}

// toParamString serializes a JSON-decoded Go value into a string argument suitable for $N::<formatType>.
func toParamString(val any) (any, error) {
	if val == nil {
		return nil, nil
	}
	switch v := val.(type) {
	case string:
		return v, nil
	case bool:
		return strconv.FormatBool(v), nil
	case float64:
		// Preserve exact integer representation if integral
		if v == float64(int64(v)) {
			return strconv.FormatInt(int64(v), 10), nil
		}
		return strconv.FormatFloat(v, 'f', -1, 64), nil
	case int64:
		return strconv.FormatInt(v, 10), nil
	case int:
		return strconv.Itoa(v), nil
	case map[string]any, []any:
		raw, err := json.Marshal(v)
		if err != nil {
			return nil, err
		}
		return string(raw), nil
	default:
		return fmt.Sprintf("%v", v), nil
	}
}
