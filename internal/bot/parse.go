package bot

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/o111oo11o/trip-ledger/pkg/currency"
)

// parseAmount parses a decimal string like "45.50" into cents (int64).
func parseAmount(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty amount")
	}

	parts := strings.SplitN(s, ".", 2)
	whole, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || whole < 0 {
		return 0, fmt.Errorf("invalid amount: %s", s)
	}

	var frac int64
	if len(parts) == 2 {
		fracStr := parts[1]
		if len(fracStr) > 2 {
			fracStr = fracStr[:2]
		}
		if len(fracStr) == 1 {
			fracStr += "0"
		}
		frac, err = strconv.ParseInt(fracStr, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid amount: %s", s)
		}
	}

	return whole*100 + frac, nil
}

// parseDate parses "dd-mm-yy" or "dd-mm" format, returns the date.
func parseDate(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	parts := strings.Split(s, "-")
	switch len(parts) {
	case 2:
		// dd-mm, use current year
		t, err := time.Parse("02-01", s)
		if err != nil {
			return time.Time{}, fmt.Errorf("invalid date: %s (use dd-mm or dd-mm-yy)", s)
		}
		return t.AddDate(time.Now().Year(), 0, 0), nil
	case 3:
		// dd-mm-yy
		t, err := time.Parse("02-01-06", s)
		if err != nil {
			return time.Time{}, fmt.Errorf("invalid date: %s (use dd-mm or dd-mm-yy)", s)
		}
		return t, nil
	default:
		return time.Time{}, fmt.Errorf("invalid date: %s (use dd-mm or dd-mm-yy)", s)
	}
}

// isUsername checks if a token looks like @username.
func isUsername(s string) bool {
	return len(s) > 1 && s[0] == '@'
}

// stripAt removes the leading @ from a username.
func stripAt(s string) string {
	return strings.TrimPrefix(s, "@")
}

// isCurrency checks if a token is a valid 3-letter currency code.
func isCurrency(s string) bool {
	return currency.IsValidCurrency(strings.ToUpper(s))
}

// isAmount checks if a token looks like a numeric amount.
func isAmount(s string) bool {
	_, err := parseAmount(s)
	return err == nil
}

// isDate checks if a token looks like a date.
func isDate(s string) bool {
	_, err := parseDate(s)
	return err == nil
}

// tokenize splits the command arguments (everything after the command itself).
func tokenize(text string) []string {
	// Remove the /command part.
	parts := strings.Fields(text)
	if len(parts) > 0 && strings.HasPrefix(parts[0], "/") {
		parts = parts[1:]
	}
	return parts
}

// joinOrDefault joins args[idx:] with spaces, returning fallback if the result is empty.
func joinOrDefault(args []string, idx int, fallback string) string {
	s := strings.Join(args[idx:], " ")
	if s == "" {
		return fallback
	}
	return s
}

// parseCurrencyAndDesc reads an optional currency code at args[idx], then joins the
// remaining tokens as a description (using fallbackDesc if none remain).
// Returns the resolved currency, description, and the advanced index.
func parseCurrencyAndDesc(args []string, idx int, defaultCurr, fallbackDesc string) (curr, desc string, next int) {
	curr = defaultCurr
	if idx < len(args) && isCurrency(args[idx]) {
		curr = strings.ToUpper(args[idx])
		idx++
	}
	return curr, joinOrDefault(args, idx, fallbackDesc), idx
}
