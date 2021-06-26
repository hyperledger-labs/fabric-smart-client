/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

//go:generate counterfeiter -o mock/session.go -fake-name Session . Session

// Session encapsulates a communication channel to an endpoint
type Session interface {
	view.Session
}
