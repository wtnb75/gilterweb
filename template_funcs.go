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
		"default":  tplDefault,
		"coalesce": tplCoalesce,
		"required": tplRequired,
		"dig":      tplDig,
		"toJson":   tplToJSON,
		"urlquery": tplURLQuery,
		"trim":     strings.TrimSpace,
		"lower":    strings.ToLower,
		"join":     strings.Join,
		"sha256":   tplSHA256,
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

func tplSHA256(v any) string {
	h := sha256.Sum256(fmt.Append(nil, v))
	return hex.EncodeToString(h[:])
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
