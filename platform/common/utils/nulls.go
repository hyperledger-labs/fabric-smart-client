/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package utils

import "reflect"

func Zero[A any]() A {
	var a A
	return a
}

func MustGet[V any](v V, err error) V {
	if err != nil {
		panic(err)
	}
	return v
}

func Must(err error) {
	if err != nil {
		panic(err)
	}
}

func DefaultZero[A any](v interface{}) A {
	var a A
	if v == nil {
		return a
	}
	if a, ok := v.(A); ok {
		return a
	}
	return a
}

func DefaultInt(a interface{}, b int) int {
	if a == nil {
		return b
	}
	if i, ok := a.(int); ok && i > 0 {
		return i
	}
	return b
}

func DefaultString(a interface{}, b string) string {
	if a == nil {
		return b
	}
	if s, ok := a.(string); ok && len(s) > 0 {
		return s
	}
	return b
}

func IsNil[T any](value T) bool {
	// Use reflection to check if the value is nil
	v := reflect.ValueOf(value)
	return (v.Kind() == reflect.Ptr || v.Kind() == reflect.Slice || v.Kind() == reflect.Map || v.Kind() == reflect.Chan || v.Kind() == reflect.Func || v.Kind() == reflect.Interface) && v.IsNil()
}
