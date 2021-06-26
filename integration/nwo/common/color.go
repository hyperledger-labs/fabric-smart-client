/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

import "fmt"

var colorIndex uint

func NextColor() string {
	color := colorIndex%14 + 31
	if color > 37 {
		color = color + 90 - 37
	}

	colorIndex++
	return fmt.Sprintf("%dm", color)
}
