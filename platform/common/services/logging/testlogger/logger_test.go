/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package testlogger_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
)

func TestMustGetLogger_WithParams(t *testing.T) {
	t.Parallel()
	const expectedSuffix = "fsc.platform.common.services.logging.testlogger_test.some.thing"
	l := logging.MustGetLogger("some", "thing")
	require.True(t, strings.HasSuffix(l.Zap().Name(), expectedSuffix))
}

func TestMustGetLogger_WithOutParams(t *testing.T) {
	t.Parallel()
	const expectedSuffix = "fsc.platform.common.services.logging.testlogger_test"
	l := logging.MustGetLogger()
	require.True(t, strings.HasSuffix(l.Zap().Name(), expectedSuffix))
}

func level1() (string, error) {
	return level2()
}

func level2() (string, error) {
	return level3()
}

func level3() (string, error) {
	return level4()
}

func level4() (string, error) {
	return logging.GetPackageName()
}

func TestGetPackageName(t *testing.T) {
	t.Parallel()
	const expectedPkg = "github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging/testlogger_test"
	pkg, err := level1()
	require.NoError(t, err)
	require.Equal(t, expectedPkg, pkg)
}

type VersionInfoHandler struct{}

func (h *VersionInfoHandler) sendResponseLikeMethod() (string, error) {
	return level2()
}

func TestGetPackageName_FromStructMethod(t *testing.T) {
	t.Parallel()
	const expectedPkg = "github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging/testlogger_test"
	handler := &VersionInfoHandler{}
	pkg, err := handler.sendResponseLikeMethod()
	require.NoError(t, err)
	require.Equal(t, expectedPkg, pkg)
}
