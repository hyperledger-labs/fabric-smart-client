/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package utils

import (
	"maps"
	"slices"
	"strings"
)

// ReplaceLongestFirst returns s with every occurrence of a key of replacements replaced
// by its value. It scans s once from left to right and does not scan a replaced part
// again; where several keys match at the same position, the longest one wins. The
// result does not depend on the iteration order of replacements.
func ReplaceLongestFirst(s string, replacements map[string]string) string {
	keys := slices.SortedFunc(maps.Keys(replacements), func(a, b string) int { return len(b) - len(a) })
	oldnew := make([]string, 0, 2*len(keys))
	for _, k := range keys {
		oldnew = append(oldnew, k, replacements[k])
	}
	return strings.NewReplacer(oldnew...).Replace(s)
}
