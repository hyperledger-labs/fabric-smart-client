/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package logging

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"log/slog"
	"reflect"
	"strings"
	"time"
	"unicode"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections"
)

const (
	nonPrintablePrefix = "[nonprintable] "
)

// Keys returns a [fmt.Stringer] that prints the keys of m, separated by commas, when it
// is formatted.
func Keys[K comparable, V any](m map[K]V) fmt.Stringer {
	return keys[K, V](m)
}

type keys[K comparable, V any] map[K]V

func (k keys[K, V]) String() string {
	return fmt.Sprintf(strings.Join(collections.Repeat("%v", len(k)), ", "), collections.Keys(k))
}

// SHA256Base64 returns a [fmt.Stringer] that prints the SHA-256 hash of b in base64 when
// it is formatted, so a disabled line doesn't hash. b must not change until then.
func SHA256Base64(b []byte) fmt.Stringer {
	return sha256Base64Enc(b)
}

type sha256Base64Enc []byte

func (b sha256Base64Enc) String() string {
	h := sha256.Sum256(b)
	return base64Enc(h[:]).String()
}

// Base64 returns a [fmt.Stringer] that prints b in base64 when it is formatted.
func Base64(b []byte) fmt.Stringer {
	return base64Enc(b)
}

type base64Enc []byte

func (b base64Enc) String() string {
	return base64.StdEncoding.EncodeToString(b)
}

// Identifier returns a [fmt.Stringer] that prints the package path and name of the type
// of f, after dereferencing pointers, when it is formatted.
func Identifier(f any) fmt.Stringer { return identifier{f} }

type identifier struct{ f any }

func (t identifier) String() string {
	if t.f == nil {
		return "<nil>"
	}
	tt := reflect.TypeOf(t.f)
	for tt.Kind() == reflect.Pointer {
		tt = tt.Elem()
	}
	return tt.PkgPath() + "/" + tt.Name()
}

// Eval returns a [fmt.Stringer] that calls f and prints its result when it is formatted.
func Eval[V any](f func() V) fmt.Stringer { return eval[V](f) }

type eval[V any] func() V

func (e eval[V]) String() string { return fmt.Sprintf("%v", e()) }

// Since returns a [fmt.Stringer] that prints the time elapsed since t when it is
// formatted.
func Since(t time.Time) fmt.Stringer { return since(t) }

type since time.Time

func (s since) String() string {
	return time.Since(time.Time(s)).String()
}

type prefix string

// Prefix returns a [fmt.Stringer] that prints id without non-printable characters. If
// that is longer than 20 characters, it prints the first 20, "~" and the base64 SHA-256
// hash of the whole.
func Prefix(id string) fmt.Stringer {
	return prefix(id)
}

func (w prefix) String() string {
	s := string(w)
	res, _ := filterPrintable(s)

	if len(res) <= 20 {
		return res
	}
	return fmt.Sprintf("%s~%s", res[:20], SHA256Base64([]byte(res)))
}

type printable string

// Printable returns a [fmt.Stringer] that prints id without non-printable characters.
func Printable(id string) fmt.Stringer {
	return printable(id)
}

func (w printable) String() string {
	s := string(w)
	res, _ := filterPrintable(s)
	return res
}

// The values the helpers above return are also [slog.LogValuer]s with the value of
// String, so a structured handler, which may not call String, logs the same text when
// one is passed as a field.

func (k keys[K, V]) LogValue() slog.Value      { return slog.StringValue(k.String()) }
func (b sha256Base64Enc) LogValue() slog.Value { return slog.StringValue(b.String()) }
func (b base64Enc) LogValue() slog.Value       { return slog.StringValue(b.String()) }
func (t identifier) LogValue() slog.Value      { return slog.StringValue(t.String()) }
func (e eval[V]) LogValue() slog.Value         { return slog.StringValue(e.String()) }
func (s since) LogValue() slog.Value           { return slog.StringValue(s.String()) }
func (w prefix) LogValue() slog.Value          { return slog.StringValue(w.String()) }
func (w printable) LogValue() slog.Value       { return slog.StringValue(w.String()) }

// filterPrintableWithMarker removes non-printable characters from the passed string.
// If non-printable characters are found, then a prefix is added to the result.
func filterPrintableWithMarker(s string) string {
	cleaned, removed := filterPrintable(s)
	if removed {
		return nonPrintablePrefix + cleaned
	}
	return cleaned
}

// filterPrintable removes non-printable characters from the passed string.
// It returns true, if characters have been removed, false otherwise.
func filterPrintable(s string) (string, bool) {
	// filter
	found := false
	cleaned := strings.Map(func(r rune) rune {
		if unicode.IsPrint(r) {
			return r
		}
		found = true
		return -1
	}, s)
	return cleaned, found
}
