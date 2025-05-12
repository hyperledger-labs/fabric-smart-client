/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package testlogger_test

import (
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	. "github.com/onsi/gomega"
)

func TestMustGetLogger_WithParams(t *testing.T) {
	RegisterTestingT(t)
	l := logging.MustGetLogger("some", "thing")

	name := l.Zap().Name()

	Expect(name).To(HaveSuffix("fsc.common.services.logging.testlogger_test.some.thing"))
}

func TestMustGetLogger_WithOutParams(t *testing.T) {
	RegisterTestingT(t)
	l := logging.MustGetLogger()

	name := l.Zap().Name()

	Expect(name).To(HaveSuffix("fsc.common.services.logging.testlogger_test"))
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
	RegisterTestingT(t)

	pkg, err := level1()
	Expect(err).NotTo(HaveOccurred())

	Expect(pkg).To(BeIdenticalTo("github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging/testlogger_test"))
}

type VersionInfoHandler struct{}

func (h *VersionInfoHandler) sendResponseLikeMethod() (string, error) {
	return level2()
}

func TestGetPackageName_FromStructMethod(t *testing.T) {
	RegisterTestingT(t)

	handler := &VersionInfoHandler{}
	pkg, err := handler.sendResponseLikeMethod()
	Expect(err).NotTo(HaveOccurred())

	Expect(pkg).To(BeIdenticalTo("github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging/testlogger_test"))
}
