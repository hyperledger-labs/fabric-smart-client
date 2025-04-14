/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package postgres

import (
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/unicode"
)

func NewSanitizer() *stringSanitizer {
	return &stringSanitizer{
		encoder: unicode.UTF8.NewEncoder(),
		decoder: unicode.UTF8.NewDecoder(),
	}
}

var replacements = map[string]string{
	"\x00":               "?0?",
	string(utf8.MaxRune): "?1?",
}

type stringSanitizer struct {
	encoder *encoding.Encoder
	decoder *encoding.Decoder
}

func (s *stringSanitizer) Encode(str string) (string, error) {
	res, err := s.encoder.String(str)
	if err != nil {
		return "", err
	}
	for original, escaped := range replacements {
		res = strings.ReplaceAll(res, original, escaped)
	}
	return res, nil
}

func (s *stringSanitizer) Decode(str string) (string, error) {
	res := str
	for original, escaped := range replacements {
		res = strings.ReplaceAll(res, escaped, original)
	}
	return s.decoder.String(res)
}
