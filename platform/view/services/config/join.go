/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package config

import (
	"strings"
)

// Join joins multiple string components with a dot (.).
// It also trims leading/trailing dots from each component to ensure clean joining.
func Join(components ...string) string {
	var validComponents []string
	for _, comp := range components {
		// Trim leading/trailing dots and spaces from each component
		trimmedComp := strings.Trim(comp, " .") // Trim both dots and spaces
		if trimmedComp != "" {
			validComponents = append(validComponents, trimmedComp)
		}
	}
	return strings.Join(validComponents, ".")
}
