/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package logging

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"reflect"
	"strings"
	"time"
	"unicode"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections"
)

const (
	nonPrintablePrefix = "[nonprintable] "
)

// Keys logs lazily the keys of a map
func Keys[K comparable, V any](m map[K]V) fmt.Stringer {
	return keys[K, V](m)
}

type keys[K comparable, V any] map[K]V

func (k keys[K, V]) String() string {
	return fmt.Sprintf(strings.Join(collections.Repeat("%v", len(k)), ", "), collections.Keys(k))
}

// SHA256Base64 logs lazily a byte slice as sha265 hash in base64 format
func SHA256Base64(b []byte) sha256Base64Enc {
	h := sha256.Sum256(b)
	return h[:]
}

type sha256Base64Enc []byte

func (b sha256Base64Enc) String() string {
	return Base64(b).String()
}

// Base64 logs lazily a byte array in base64 format
func Base64(b []byte) base64Enc {
	return b
}

type base64Enc []byte

func (b base64Enc) String() string {
	return base64.StdEncoding.EncodeToString(b)
}

// Identifier logs lazily the identifier of any object
func Identifier(f any) identifier { return identifier{f} }

type identifier struct{ f any }

func (t identifier) String() string {
	if t.f == nil {
		return "<nil>"
	}
	tt := reflect.TypeOf(t.f)
	for tt.Kind() == reflect.Ptr {
		tt = tt.Elem()
	}
	return tt.PkgPath() + "/" + tt.Name()
}

func Eval[V any](f func() V) eval[V] { return f }

type eval[V any] func() V

func (e eval[V]) String() string { return fmt.Sprintf("%v", e()) }

func Since(t time.Time) since { return since(t) }

type since time.Time

func (s since) String() string {
	return time.Since(time.Time(s)).String()
}

type prefix string

func Prefix(id string) fmt.Stringer {
	return prefix(id)
}

func (w prefix) String() string {
	s := string(w)
	res, _ := FilterPrintable(s)

	if len(res) <= 20 {
		return res
	}
	return fmt.Sprintf("%s~%s", res[:20], SHA256Base64([]byte(res)))
}

type printable string

func Printable(id string) fmt.Stringer {
	return printable(id)
}

func (w printable) String() string {
	s := string(w)
	res, _ := FilterPrintable(s)
	return res
}

// FilterPrintableWithMarker removes non-printable characters from the passed string.
// If non-printable characters are found, then a prefix is added to the result.
func FilterPrintableWithMarker(s string) string {
	cleaned, removed := FilterPrintable(s)
	if removed {
		return nonPrintablePrefix + cleaned
	}
	return cleaned
}

// FilterPrintable removes non-printable characters from the passed string.
// It returns true, if characters have been removed, false otherwise.
func FilterPrintable(s string) (string, bool) {
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
