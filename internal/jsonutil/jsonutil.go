// Package jsonutil provides helper functions for extracting typed values
// from unstructured JSON maps (map[string]any).
package jsonutil

import "encoding/json"

// IntFromAny converts various numeric types to int.
func IntFromAny(value interface{}) int {
	switch num := value.(type) {
	case float64:
		return int(num)
	case int:
		return num
	case int64:
		return int(num)
	case json.Number:
		i, _ := num.Int64()
		return int(i)
	default:
		return 0
	}
}

// FloatFromAny converts various numeric types to float64.
func FloatFromAny(v interface{}) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	case int64:
		return float64(n)
	case json.Number:
		f, _ := n.Float64()
		return f
	default:
		return 0
	}
}

// StringFromAny safely converts any value to string.
func StringFromAny(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// IntFromMap extracts an int from a map by key.
func IntFromMap(data map[string]interface{}, key string) int {
	if v, ok := data[key]; ok {
		return IntFromAny(v)
	}
	return 0
}

// StringFromMap extracts a string from a map by key.
func StringFromMap(data map[string]interface{}, key string) string {
	if v, ok := data[key]; ok {
		return StringFromAny(v)
	}
	return ""
}

// BoolFromMap extracts a bool from a map by key.
func BoolFromMap(data map[string]interface{}, key string) bool {
	if v, ok := data[key]; ok {
		if boolVal, ok := v.(bool); ok {
			return boolVal
		}
	}
	return false
}
