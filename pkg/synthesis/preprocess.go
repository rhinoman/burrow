package synthesis

import (
	"encoding/json"
	"fmt"
	"strings"
)

// metadataKeys are JSON keys skipped during preprocessing because they contain
// metadata noise rather than user-relevant content.
var metadataKeys = map[string]bool{
	"icon":            true,
	"@context":        true,
	"@type":           true,
	"@id":             true,
	"unitCode":        true,
	"correlationId":   true,
	"correlationid":   true,
	"requestId":       true,
	"requestid":       true,
	"pagination":      true,
	"geometry":        true,
	"forecastOffice":  true,
	"gridId":          true,
	"gridX":           true,
	"gridY":           true,
	"cwa":             true,
	"radarStation":    true,
	"timeZone":        true,
	"generatedAt":     true,
	"updateTime":      true,
	"updated":         true,
	"elevation":       true,
	"bearingAndRange": true,
}

// PreprocessData converts raw JSON service data into a compact, token-efficient
// text representation for LLM consumption. Non-JSON input is returned unchanged.
//
// The algorithm:
//  1. Try JSON parse. If it fails, return data unchanged (handles plain text, XML).
//  2. Recursively flatten JSON to indented key-value text.
//  3. Skip known metadata keys that add noise.
//  4. Unwrap unit-code wrapper objects: {"value": 67, "unitCode": "..."} → 67.
func PreprocessData(data string) string {
	data = strings.TrimSpace(data)
	if data == "" {
		return data
	}

	// Try parsing as JSON.
	var parsed interface{}
	if err := json.Unmarshal([]byte(data), &parsed); err != nil {
		return data // not JSON — return unchanged
	}

	var b strings.Builder
	flattenValue(&b, parsed, 0)
	result := strings.TrimRight(b.String(), "\n")
	if result == "" {
		return data // degenerate case — return original
	}
	return result
}

// flattenValue recursively renders a JSON value as indented text.
func flattenValue(b *strings.Builder, v interface{}, indent int) {
	switch val := v.(type) {
	case map[string]interface{}:
		flattenObject(b, val, indent)
	case []interface{}:
		flattenArray(b, val, indent)
	default:
		// Scalar value
		writeIndent(b, indent)
		b.WriteString(formatScalar(val))
		b.WriteByte('\n')
	}
}

// flattenObject renders a JSON object as labeled key-value lines.
func flattenObject(b *strings.Builder, obj map[string]interface{}, indent int) {
	// Check for unit-code wrapper pattern: {"value": X, "unitCode": "..."}
	if unwrapped, ok := unwrapUnitCode(obj); ok {
		writeIndent(b, indent)
		b.WriteString(formatScalar(unwrapped))
		b.WriteByte('\n')
		return
	}

	for key, val := range obj {
		if metadataKeys[key] {
			continue
		}

		switch child := val.(type) {
		case map[string]interface{}:
			// Check for unit-code wrapper in nested object
			if unwrapped, ok := unwrapUnitCode(child); ok {
				writeIndent(b, indent)
				b.WriteString(key)
				b.WriteString(": ")
				b.WriteString(formatScalar(unwrapped))
				b.WriteByte('\n')
			} else {
				writeIndent(b, indent)
				b.WriteString(key)
				b.WriteString(":\n")
				flattenObject(b, child, indent+1)
			}
		case []interface{}:
			writeIndent(b, indent)
			b.WriteString(key)
			b.WriteString(":\n")
			flattenArray(b, child, indent+1)
		default:
			writeIndent(b, indent)
			b.WriteString(key)
			b.WriteString(": ")
			b.WriteString(formatScalar(val))
			b.WriteByte('\n')
		}
	}
}

// flattenArray renders a JSON array as numbered items.
func flattenArray(b *strings.Builder, arr []interface{}, indent int) {
	for i, item := range arr {
		switch child := item.(type) {
		case map[string]interface{}:
			// Use a meaningful label if available, otherwise number.
			label := extractLabel(child)
			if label != "" {
				writeIndent(b, indent)
				b.WriteString(label)
				b.WriteString(":\n")
			} else {
				writeIndent(b, indent)
				b.WriteString(fmt.Sprintf("%d.", i+1))
				b.WriteByte('\n')
			}
			flattenObject(b, child, indent+1)
		case []interface{}:
			writeIndent(b, indent)
			b.WriteString(fmt.Sprintf("%d.", i+1))
			b.WriteByte('\n')
			flattenArray(b, child, indent+1)
		default:
			writeIndent(b, indent)
			b.WriteString(fmt.Sprintf("%d. ", i+1))
			b.WriteString(formatScalar(item))
			b.WriteByte('\n')
		}
	}
}

// labelKeys are checked in order to find a human-readable label for array items.
var labelKeys = []string{"name", "title", "label", "number", "detailedForecast"}

// extractLabel returns a short display label from an object, or "" if none found.
// Used to produce readable section headers for array items instead of bare numbers.
func extractLabel(obj map[string]interface{}) string {
	// Try "name" field first for weather period names like "Tonight", "Monday"
	if name, ok := obj["name"]; ok {
		s := formatScalar(name)
		if s != "" && s != "null" {
			return s
		}
	}
	// Try "title" for articles/items
	if title, ok := obj["title"]; ok {
		s := formatScalar(title)
		if s != "" && s != "null" {
			return s
		}
	}
	return ""
}

// unwrapUnitCode detects the NWS-style value wrapper pattern:
// {"value": X, "unitCode": "wmoUnit:..."} and returns just X.
func unwrapUnitCode(obj map[string]interface{}) (interface{}, bool) {
	if len(obj) != 2 {
		return nil, false
	}
	val, hasValue := obj["value"]
	_, hasUnit := obj["unitCode"]
	if hasValue && hasUnit {
		return val, true
	}
	return nil, false
}

// formatScalar converts a scalar JSON value to its string representation.
func formatScalar(v interface{}) string {
	switch val := v.(type) {
	case string:
		return val
	case float64:
		// Format as integer if no fractional part.
		if val == float64(int64(val)) {
			return fmt.Sprintf("%d", int64(val))
		}
		return fmt.Sprintf("%g", val)
	case bool:
		if val {
			return "true"
		}
		return "false"
	case nil:
		return ""
	default:
		return fmt.Sprintf("%v", val)
	}
}

// writeIndent writes indent*2 spaces to the builder.
func writeIndent(b *strings.Builder, indent int) {
	for i := 0; i < indent; i++ {
		b.WriteString("  ")
	}
}
