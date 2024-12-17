/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package diag_test

import (
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/node/node/diag"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
)

func TestCaptureGoRoutines(t *testing.T) {
	gt := NewGomegaWithT(t)
	output, err := diag.CaptureGoRoutines()
	gt.Expect(err).NotTo(HaveOccurred())

	gt.Expect(output).To(MatchRegexp(`goroutine \d+ \[running\]:`))
	gt.Expect(output).To(ContainSubstring("github.com/hyperledger-labs/fabric-smart-client/node/node/diag.CaptureGoRoutines"))
}

func TestLogGoRoutines(t *testing.T) {
	gt := NewGomegaWithT(t)
	logger, recorder := logging.NewTestLogger(t, logging.Named("goroutine"))
	diag.LogGoRoutines(logger)

	gt.Expect(recorder).To(gbytes.Say(`goroutine \d+ \[running\]:`))
}
