package importer

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

func parseOptionalInt(value string, field string) (*int, error) {
	cleaned := strings.TrimSpace(value)
	if cleaned == "" {
		return nil, nil
	}
	parsed, err := strconv.Atoi(cleaned)
	if err != nil {
		return nil, fmt.Errorf("%s must be a number", field)
	}
	if parsed <= 0 {
		return nil, fmt.Errorf("%s must be positive", field)
	}
	return &parsed, nil
}

func parseOptionalNonNegativeInt(value string, field string) (*int, error) {
	cleaned := strings.TrimSpace(value)
	if cleaned == "" {
		return nil, nil
	}
	parsed, err := strconv.Atoi(cleaned)
	if err != nil {
		return nil, fmt.Errorf("%s must be a number", field)
	}
	if parsed < 0 {
		return nil, fmt.Errorf("%s must be zero or greater", field)
	}
	return &parsed, nil
}

func parseOptionalIntAllowAny(value string, field string) (*int, error) {
	cleaned := strings.TrimSpace(value)
	if cleaned == "" {
		return nil, nil
	}
	parsed, err := strconv.Atoi(cleaned)
	if err != nil {
		return nil, fmt.Errorf("%s must be a number", field)
	}
	return &parsed, nil
}

func parseOptionalFloat(value string, field string) (*float64, error) {
	cleaned := strings.TrimSpace(value)
	if cleaned == "" {
		return nil, nil
	}
	parsed, err := strconv.ParseFloat(cleaned, 64)
	if err != nil || math.IsNaN(parsed) || math.IsInf(parsed, 0) {
		return nil, fmt.Errorf("%s must be a finite number", field)
	}
	return &parsed, nil
}

func parseOptionalTime(value string, field string) (*time.Time, error) {
	cleaned := strings.TrimSpace(value)
	if cleaned == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339, cleaned)
	if err != nil {
		parsed, err = time.Parse(time.RFC3339Nano, cleaned)
	}
	if err != nil {
		return nil, fmt.Errorf("%s must be an RFC3339 timestamp", field)
	}
	return &parsed, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
