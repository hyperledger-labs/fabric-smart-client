/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sql

type Sanitizer interface {
	Encode(string) (string, error)
	Decode(string) (string, error)
}
