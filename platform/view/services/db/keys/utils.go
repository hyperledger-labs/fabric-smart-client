/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package keys

import (
	"regexp"
	"unicode/utf8"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/pkg/errors"
)

var (
	nsRegexp  = regexp.MustCompile("^[a-zA-Z0-9._-]{1,128}$")
	keyRegexp = regexp.MustCompile("^[a-zA-Z0-9._~\u0000=" + string(utf8.MaxRune) + "+/-]{1,}$")
)

const NamespaceSeparator = "\u0000"

func ValidateKey(key string) error {
	if !keyRegexp.MatchString(key) {
		return errors.Errorf("key '%s' is invalid", key)
	}
	// TODO: should we enforce a length limit?

	return nil
}

func ValidateNs(ns string) error {
	if !nsRegexp.MatchString(ns) {
		return errors.Errorf("namespace '%s' is invalid", ns)
	}

	return nil
}

type DummyVersionedIterator struct {
	idx   int
	Items []*driver.VersionedRead
}

func (r *DummyVersionedIterator) Next() (*driver.VersionedRead, error) {
	if r.Items == nil || r.idx == len(r.Items) {
		return nil, nil
	}
	r.idx++
	return r.Items[r.idx-1], nil
}

func (r *DummyVersionedIterator) Close() {}
