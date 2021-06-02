/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package fabric

import "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/api"

type Committer struct {
	ch api.Channel
}

func (c *Committer) ProcessNamespace(nss ...string) error {
	return c.ch.ProcessNamespace(nss...)
}
