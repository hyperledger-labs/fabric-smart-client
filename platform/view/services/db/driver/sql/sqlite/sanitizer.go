/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sqlite

func newSanitizer() *stringSanitizer { return &stringSanitizer{} }

type stringSanitizer struct{}

func (s *stringSanitizer) Encode(str string) (string, error) { return str, nil }

func (s *stringSanitizer) Decode(str string) (string, error) { return str, nil }
