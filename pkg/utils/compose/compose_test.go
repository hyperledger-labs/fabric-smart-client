/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package compose

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAppendAttributes(t *testing.T) {
	var sb strings.Builder
	CreateCompositeKeyOrPanic(&sb, "ot", "1", "2")
	k := AppendAttributesOrPanic(&sb, "3")
	assert.Equal(t, CreateCompositeKeyOrPanic(&strings.Builder{}, "ot", "1", "2", "3"), k)
}
