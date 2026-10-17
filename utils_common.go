package main

import (
	"crypto/rand"
	"encoding/json"
	"math/big"
	"reflect"
	"strings"
)

func RandInt(min int, max int) int {
	if max < min {
		return min
	}
	b, err := rand.Int(rand.Reader, big.NewInt(int64(max-min+1)))
	if err != nil {
		return min
	}
	return min + int(b.Int64())
}

func StringInSlice(a string, list []string) bool {
	for _, b := range list {
		if b == a {
			return true
		}
	}
	return false
}

// MarshalOmitEmptyAll converts a struct to JSON, dropping ALL zero-value fields.
func MarshalOmitEmptyAll(v any) ([]byte, error) {
	val := reflect.ValueOf(v)

	// Handle pointers
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}

	// If it's not a struct, fall back to standard marshaling
	if val.Kind() != reflect.Struct {
		return json.Marshal(v)
	}

	resultMap := make(map[string]any)
	t := val.Type()

	for i := 0; i < val.NumField(); i++ {
		fieldVal := val.Field(i)
		fieldType := t.Field(i)

		// Skip unexported (private) fields
		if !fieldType.IsExported() {
			continue
		}

		// Get the JSON key name from the tag
		jsonTag := fieldType.Tag.Get("json")
		if jsonTag == "-" {
			continue
		}

		key := fieldType.Name
		if jsonTag != "" {
			// Split in case it has options like ,string
			parts := strings.Split(jsonTag, ",")
			if parts[0] != "" {
				key = parts[0]
			}
		}

		// Drop the field if it matches its type's zero value
		if fieldVal.IsZero() {
			continue
		}

		// Recursively handle nested structs or arrays of structs
		if fieldVal.Kind() == reflect.Struct {
			nestedBytes, err := MarshalOmitEmptyAll(fieldVal.Interface())
			if err != nil {
				return nil, err
			}
			var nestedMap map[string]any
			if err := json.Unmarshal(nestedBytes, &nestedMap); err == nil {
				resultMap[key] = nestedMap
				continue
			}
		}

		resultMap[key] = fieldVal.Interface()
	}

	return json.Marshal(resultMap)
}
