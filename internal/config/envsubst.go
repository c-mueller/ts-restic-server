package config

import (
	"fmt"
	"os"
	"reflect"
	"regexp"
)

var envVarPattern = regexp.MustCompile(`\$\{([^}]+)\}`)

// ResolveEnvVars walks all string fields in the given struct (including nested
// structs, pointers, and string slices) and replaces ${VAR_NAME} placeholders
// with the corresponding environment variable value.
//
// When lenient is false (strict mode), an error is returned if any referenced
// variable is not set. When lenient is true, unresolved placeholders are left
// as-is.
func ResolveEnvVars(v interface{}, lenient bool) error {
	return resolveValue(reflect.ValueOf(v), lenient)
}

func resolveValue(v reflect.Value, lenient bool) error {
	switch v.Kind() {
	case reflect.Ptr:
		if v.IsNil() {
			return nil
		}
		return resolveValue(v.Elem(), lenient)

	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			field := v.Field(i)
			if !field.CanSet() {
				continue
			}
			if err := resolveValue(field, lenient); err != nil {
				return err
			}
		}

	case reflect.String:
		resolved, err := substituteString(v.String(), lenient)
		if err != nil {
			return err
		}
		v.SetString(resolved)

	case reflect.Slice:
		for i := 0; i < v.Len(); i++ {
			if err := resolveValue(v.Index(i), lenient); err != nil {
				return err
			}
		}
	}

	return nil
}

func substituteString(s string, lenient bool) (string, error) {
	var errOut error
	result := envVarPattern.ReplaceAllStringFunc(s, func(match string) string {
		if errOut != nil {
			return match
		}
		name := envVarPattern.FindStringSubmatch(match)[1]
		val, ok := os.LookupEnv(name)
		if !ok {
			if !lenient {
				errOut = fmt.Errorf("environment variable %q not set (referenced in config)", name)
			}
			return match
		}
		return val
	})
	return result, errOut
}
