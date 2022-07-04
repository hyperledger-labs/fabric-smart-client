//go:build tools

/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package tools

import (
	_ "github.com/client9/misspell/cmd/misspell"
	_ "github.com/google/addlicense"
	_ "github.com/gordonklaus/ineffassign"
	_ "github.com/onsi/ginkgo/ginkgo"
)
