/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

import (
	"regexp"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
)

// plainIdentifier matches a plain SQL identifier: a letter or underscore
// followed by letters, digits or underscores. Go's \w is ASCII-only, so it does
// not widen the character class beyond that.
var plainIdentifier = regexp.MustCompile(`^[A-Za-z_]\w*$`)

// FieldName is the name of a database column.
type FieldName string

// Validate reports whether n is a plain SQL identifier.
//
// A field name is written into a statement verbatim — it is a column name, not a
// parameter, so it cannot be bound — which makes it the one part of a generated
// statement an untrusted value could rewrite. Anything a store accepts from
// outside, in particular a column name read back out of a serialized pagination
// cursor, has to pass through here before it is written.
func (n FieldName) Validate() error {
	if !plainIdentifier.MatchString(string(n)) {
		return errors.Errorf("column name %q is not a plain identifier", string(n))
	}
	return nil
}
