package engine

import (
	"fmt"
	"regexp"
	"strings"
)

var safeColumnTypeRe = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_ ]*(\s*\(\s*[0-9]+(\s*,\s*[0-9]+)?\s*\))?(\s*\[\])?$`)

// ValidateName ensures database/schema/table/column identifiers are non-empty and free of null bytes.
func ValidateName(name string) (string, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return "", fmt.Errorf("%w: identifier cannot be empty", ErrInvalidInput)
	}
	if len(trimmed) > 128 {
		return "", fmt.Errorf("%w: identifier exceeds 128 characters", ErrInvalidInput)
	}
	if strings.ContainsRune(trimmed, '\x00') {
		return "", fmt.Errorf("%w: identifier contains null byte", ErrInvalidInput)
	}
	return trimmed, nil
}

// ValidateColumnType validates a SQL column type string used in CREATE TABLE.
func ValidateColumnType(colType string) (string, error) {
	trimmed := strings.TrimSpace(colType)
	if !safeColumnTypeRe.MatchString(trimmed) {
		return "", fmt.Errorf("%w: invalid column data type %q", ErrInvalidInput, trimmed)
	}
	return trimmed, nil
}

func containsUnsafeSQLTokens(s string) bool {
	return strings.ContainsRune(s, ';') || strings.Contains(s, "--") || strings.Contains(s, "/*") || strings.ContainsRune(s, '\x00')
}

// ValidateDefaultExpr rejects comment/statement tokens in column DEFAULT expressions.
func ValidateDefaultExpr(expr string) (string, error) {
	trimmed := strings.TrimSpace(expr)
	if trimmed == "" {
		return "", nil
	}
	if containsUnsafeSQLTokens(trimmed) {
		return "", fmt.Errorf("%w: invalid default expression %q", ErrInvalidInput, trimmed)
	}
	return trimmed, nil
}

// ValidateCheckExpr rejects empty or unsafe CHECK expressions.
func ValidateCheckExpr(expr string) (string, error) {
	trimmed := strings.TrimSpace(expr)
	if trimmed == "" || containsUnsafeSQLTokens(trimmed) {
		return "", fmt.Errorf("%w: invalid CHECK expression %q", ErrInvalidInput, trimmed)
	}
	return trimmed, nil
}

// ValidateIndexMethod validates PostgreSQL index access methods.
func ValidateIndexMethod(method string) (string, error) {
	m := strings.ToLower(strings.TrimSpace(method))
	switch m {
	case "", "btree", "hash", "gin", "gist", "spgist", "brin":
		return m, nil
	default:
		return "", fmt.Errorf("%w: unsupported index method %q", ErrInvalidInput, method)
	}
}

// ValidateFKAction validates ON DELETE / ON UPDATE referential actions.
func ValidateFKAction(action string) (string, error) {
	a := strings.ToUpper(strings.TrimSpace(action))
	switch a {
	case "", "NO ACTION", "RESTRICT", "CASCADE", "SET NULL", "SET DEFAULT":
		return a, nil
	default:
		return "", fmt.Errorf("%w: unsupported referential action %q", ErrInvalidInput, action)
	}
}
