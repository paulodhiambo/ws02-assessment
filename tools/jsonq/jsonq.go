// Package jsonq evaluates small dotted paths against a JSON document, for
// shell scripts that need one value out of an API response.
//
// Path syntax (segments separated by '.'):
//
//	name            object field
//	name[2]         array index
//	name[*]         every element (one result per line)
//	name[key=val]   first element whose field key equals val
//
// e.g. "error.code", "errors[0].code", "values[key=apiKey].value", "list[*].url".
package jsonq

import (
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
)

var segment = regexp.MustCompile(`^([^\[\]]*)((?:\[[^\]]*\])*)$`)
var selector = regexp.MustCompile(`\[([^\]]*)\]`)

// Query returns every value the path selects, formatted for shell use:
// strings unquoted, numbers and booleans as written, objects/arrays as JSON.
func Query(r io.Reader, path string) ([]string, error) {
	var doc any
	dec := json.NewDecoder(r)
	dec.UseNumber()
	if err := dec.Decode(&doc); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}
	current := []any{doc}
	if path != "" && path != "." {
		for _, seg := range strings.Split(path, ".") {
			m := segment.FindStringSubmatch(seg)
			if m == nil {
				return nil, fmt.Errorf("invalid path segment %q", seg)
			}
			if m[1] != "" {
				current = field(current, m[1])
			}
			for _, sel := range selector.FindAllStringSubmatch(m[2], -1) {
				var err error
				if current, err = index(current, sel[1]); err != nil {
					return nil, err
				}
			}
		}
	}
	if len(current) == 0 {
		return nil, fmt.Errorf("path %q not found", path)
	}
	out := make([]string, 0, len(current))
	for _, v := range current {
		out = append(out, format(v))
	}
	return out, nil
}

func field(values []any, name string) []any {
	var out []any
	for _, v := range values {
		if obj, ok := v.(map[string]any); ok {
			if f, ok := obj[name]; ok {
				out = append(out, f)
			}
		}
	}
	return out
}

func index(values []any, sel string) ([]any, error) {
	var out []any
	for _, v := range values {
		arr, ok := v.([]any)
		if !ok {
			continue
		}
		switch {
		case sel == "*":
			out = append(out, arr...)
		case strings.Contains(sel, "="):
			key, want, _ := strings.Cut(sel, "=")
			for _, e := range arr {
				if obj, ok := e.(map[string]any); ok && format(obj[key]) == want {
					out = append(out, e)
					break
				}
			}
		default:
			i, err := strconv.Atoi(sel)
			if err != nil {
				return nil, fmt.Errorf("invalid selector [%s]", sel)
			}
			if i >= 0 && i < len(arr) {
				out = append(out, arr[i])
			}
		}
	}
	return out, nil
}

func format(v any) string {
	switch x := v.(type) {
	case nil:
		return "null"
	case string:
		return x
	case json.Number:
		return x.String()
	case bool:
		return strconv.FormatBool(x)
	default:
		b, _ := json.Marshal(x)
		return string(b)
	}
}
