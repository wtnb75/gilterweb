package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"reflect"
	"strings"
	"text/template"
)

func templateFuncMap() template.FuncMap {
	return template.FuncMap{
		"default":      tplDefault,
		"coalesce":     tplCoalesce,
		"required":     tplRequired,
		"dig":          tplDig,
		"toJson":       tplToJSON,
		"urlquery":     tplURLQuery,
		"trim":         tplTrim,
		"trimSpace":    tplTrimSpace,
		"trimPrefix":   tplTrimPrefix,
		"trimSuffix":   tplTrimSuffix,
		"removePrefix": tplRemovePrefix,
		"removeSuffix": tplRemoveSuffix,
		"split":        tplSplit,
		"indent":       tplIndent,
		"lower":        strings.ToLower,
		"join":         strings.Join,
		"contains":     tplContains,
		"hasPrefix":    tplHasPrefix,
		"hasSuffix":    tplHasSuffix,
		"sha256":       tplSHA256,
	}
}

func tplDefault(fallback, value any) any {
	if isEmpty(value) {
		return fallback
	}
	return value
}

func tplCoalesce(values ...any) any {
	for _, v := range values {
		if !isEmpty(v) {
			return v
		}
	}
	return ""
}

func tplRequired(msg string, value any) (any, error) {
	if isEmpty(value) {
		return nil, errors.New(msg)
	}
	return value, nil
}

func tplDig(obj any, keys ...string) any {
	cur := obj
	for _, k := range keys {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur, ok = m[k]
		if !ok {
			return nil
		}
	}
	return cur
}

func tplToJSON(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func tplURLQuery(v any) string {
	return url.QueryEscape(fmt.Sprint(v))
}

func tplTrim(cutset, s any) string {
	return strings.Trim(fmt.Sprint(s), fmt.Sprint(cutset))
}

func tplTrimSpace(s any) string {
	return strings.TrimSpace(fmt.Sprint(s))
}

func tplTrimPrefix(cutset, s any) string {
	return strings.TrimLeft(fmt.Sprint(s), fmt.Sprint(cutset))
}

func tplTrimSuffix(cutset, s any) string {
	return strings.TrimRight(fmt.Sprint(s), fmt.Sprint(cutset))
}

func tplRemovePrefix(prefix, s any) string {
	return strings.TrimPrefix(fmt.Sprint(s), fmt.Sprint(prefix))
}

func tplRemoveSuffix(suffix, s any) string {
	return strings.TrimSuffix(fmt.Sprint(s), fmt.Sprint(suffix))
}

func tplSplit(sep, s any) []string {
	return strings.Split(fmt.Sprint(s), fmt.Sprint(sep))
}

func tplIndent(width, s any) string {
	text := fmt.Sprint(s)
	n, ok := width.(int)
	if !ok {
		if x, ok := width.(int64); ok {
			n = int(x)
		} else {
			n = 0
		}
	}
	if n <= 0 || text == "" {
		return text
	}
	pad := strings.Repeat(" ", n)
	return pad + strings.ReplaceAll(text, "\n", "\n"+pad)
}

func tplSHA256(v any) string {
	h := sha256.Sum256(fmt.Append(nil, v))
	return hex.EncodeToString(h[:])
}

func tplContains(container, needle any) bool {
	if container == nil {
		return false
	}
	rv := reflect.ValueOf(container)
	switch rv.Kind() {
	case reflect.String:
		return strings.Contains(rv.String(), fmt.Sprint(needle))
	case reflect.Array, reflect.Slice:
		for i := 0; i < rv.Len(); i++ {
			if reflect.DeepEqual(rv.Index(i).Interface(), needle) {
				return true
			}
		}
	}
	return false
}

func tplHasPrefix(s, prefix any) bool {
	return strings.HasPrefix(fmt.Sprint(s), fmt.Sprint(prefix))
}

func tplHasSuffix(s, suffix any) bool {
	return strings.HasSuffix(fmt.Sprint(s), fmt.Sprint(suffix))
}

func isEmpty(v any) bool {
	if v == nil {
		return true
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.String, reflect.Array, reflect.Slice, reflect.Map:
		return rv.Len() == 0
	case reflect.Bool:
		return !rv.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return rv.Int() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return rv.Uint() == 0
	case reflect.Float32, reflect.Float64:
		return rv.Float() == 0
	case reflect.Interface, reflect.Pointer:
		return rv.IsNil()
	}
	return false
}
