package main

import (
	"fmt"
	"regexp"
	"time"
)

// validateFilterParams statically checks a filter's params for the required
// fields execFilter would otherwise only discover at request time. It only
// inspects literal config values — never template-rendered ones — so it
// cannot catch everything execFilter checks (e.g. an http.url that renders
// to empty at runtime), but it catches the common case of a missing or
// empty required field before the config is ever served.
func validateFilterParams(f FilterConfig, ids map[string]bool) error {
	switch f.Type {
	case "static":
		return nil
	case "env":
		return validateEnvParams(f)
	case "http":
		return validateHTTPParams(f)
	case "exec":
		return validateExecParams(f)
	case "file":
		return validateFileParams(f)
	case "jq":
		return validateJQParams(f)
	case "base64":
		return validateBase64Params(f)
	case "regex":
		return validateRegexParams(f)
	case "cache":
		return validateCacheParams(f, ids)
	default:
		return nil
	}
}

func validateEnvParams(f FilterConfig) error {
	params, ok := asMap(f.Params)
	if !ok {
		return fmt.Errorf("env params must be object")
	}
	if toString(params["name"]) == "" {
		return fmt.Errorf("env.name required")
	}
	return nil
}

func validateHTTPParams(f FilterConfig) error {
	params, ok := asMap(f.Params)
	if !ok {
		return fmt.Errorf("http params must be object")
	}
	if toString(params["url"]) == "" {
		return fmt.Errorf("http.url required")
	}
	return nil
}

func validateExecParams(f FilterConfig) error {
	params, ok := asMap(f.Params)
	if !ok {
		return fmt.Errorf("exec params must be object")
	}
	rawCmd, ok := params["command"].([]any)
	if !ok || len(rawCmd) == 0 {
		return fmt.Errorf("exec.command required")
	}
	for i, v := range rawCmd {
		if _, ok := v.(string); !ok {
			return fmt.Errorf("exec.command[%d] must be a string", i)
		}
	}
	return nil
}

func validateFileParams(f FilterConfig) error {
	params, ok := asMap(f.Params)
	if !ok {
		return fmt.Errorf("file params must be object")
	}
	if toString(params["path"]) == "" {
		return fmt.Errorf("file.path required")
	}
	return nil
}

func validateJQParams(f FilterConfig) error {
	params, ok := asMap(f.Params)
	if !ok {
		return fmt.Errorf("jq params must be object")
	}
	if toString(params["query"]) == "" {
		return fmt.Errorf("jq.query required")
	}
	return nil
}

func validateBase64Params(f FilterConfig) error {
	params, ok := asMap(f.Params)
	if !ok {
		return fmt.Errorf("base64 params must be object")
	}
	op, _ := params["op"].(string)
	if !inSet(op, "encode", "decode") {
		return fmt.Errorf("base64 op must be encode|decode")
	}
	return nil
}

func validateRegexParams(f FilterConfig) error {
	params, ok := asMap(f.Params)
	if !ok {
		return fmt.Errorf("regex params must be object")
	}
	op, _ := params["op"].(string)
	if !inSet(op, "find", "find_all", "replace") {
		return fmt.Errorf("regex op must be find|find_all|replace")
	}
	pattern, _ := params["pattern"].(string)
	multiline, _ := params["multiline"].(bool)
	if multiline {
		pattern = "(?m)" + pattern
	}
	if _, err := regexp.Compile(pattern); err != nil {
		return fmt.Errorf("regex.pattern invalid: %w", err)
	}
	if op == "replace" {
		if replace, _ := params["replace"].(string); replace == "" {
			return fmt.Errorf("replace required when op=replace")
		}
	}
	return nil
}

func validateCacheParams(f FilterConfig, ids map[string]bool) error {
	params, ok := asMap(f.Params)
	if !ok {
		return fmt.Errorf("cache params must be object")
	}
	target, _ := params["filter"].(string)
	if target == "" {
		return fmt.Errorf("cache.filter required")
	}
	if !ids[target] {
		return fmt.Errorf("cache.filter references undefined filter: %s", target)
	}
	if ttlStr, _ := params["ttl"].(string); ttlStr != "" {
		if _, err := time.ParseDuration(ttlStr); err != nil {
			return fmt.Errorf("cache.ttl invalid: %w", err)
		}
	}
	return nil
}
