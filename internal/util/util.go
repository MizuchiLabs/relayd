// Package util provides common utility functions for string manipulation and environment variables.
package util

import (
	"os"
	"strings"
)

// SplitCSV splits a comma-separated string into a slice of trimmed strings, ignoring empty values.
func SplitCSV(val string) []string {
	var out []string
	for p := range strings.SplitSeq(val, ",") {
		if v := strings.TrimSpace(p); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// GetEnv retrieves the value of the environment variable  with a fallback value.
func GetEnv(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

// NormalizeHostname lowercases and trims whitespace and trailing dots from a hostname.
func NormalizeHostname(value string) string {
	v := strings.TrimSpace(strings.ToLower(value))
	v = strings.TrimSuffix(v, ".")
	if strings.Contains(v, " ") {
		return ""
	}
	return v
}

// ParseQuotedValues extracts values enclosed in single quotes, double quotes, or backticks.
func ParseQuotedValues(input string) []string {
	var out []string
	runes := []rune(input)
	for i := 0; i < len(runes); i++ {
		if r := runes[i]; r == '\'' || r == '"' || r == '`' {
			for j := i + 1; j < len(runes); j++ {
				if runes[j] == r {
					if v := NormalizeHostname(string(runes[i+1 : j])); v != "" {
						out = append(out, v)
					}
					i = j
					break
				}
			}
		}
	}
	return out
}

// WithDot ensures a string ends with a trailing dot.
func WithDot(s string) string {
	if s == "" || strings.HasSuffix(s, ".") {
		return s
	}
	return s + "."
}
