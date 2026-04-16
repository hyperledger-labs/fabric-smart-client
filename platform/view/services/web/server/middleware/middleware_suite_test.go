/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package middleware_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestMiddleware(t *testing.T) { //nolint:paralleltest
	RegisterFailHandler(Fail)
	RunSpecs(t, "Middleware Suite")
}
