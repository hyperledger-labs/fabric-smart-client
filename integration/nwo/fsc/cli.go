/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fsc

import (
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc/commands"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/api"
	"github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
)

type flowCLI struct {
	timeout time.Duration
	p       *platform
	CMD     commands.Flow
}

func (f flowCLI) CallView(fid string, in []byte) (interface{}, error) {
	f.CMD.Input = string(in)
	f.CMD.Function = fid

	sess, err := f.p.Flow(f.CMD)
	if err != nil {
		return nil, err
	}
	gomega.Eventually(sess, f.timeout).Should(gexec.Exit(0))

	return string(sess.Out.Contents()), nil
}

func (f flowCLI) IsTxFinal(txid string, opts ...api.ServiceOption) error {
	panic("not implemented yet")
}
