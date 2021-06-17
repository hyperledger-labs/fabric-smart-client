/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package driver

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// Comm gives access to comm related information
type Comm interface {
	// GetTLSRootCert returns the TLS root certificates of the MSP the passed identity belongs to
	GetTLSRootCert(party view.Identity) ([][]byte, error)
}
