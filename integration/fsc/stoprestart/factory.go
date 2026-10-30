/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package stoprestart

import "github.com/hyperledger-labs/fabric-smart-client/platform/view/view"

type InitiatorViewFactory struct{}

func (*InitiatorViewFactory) NewView(_ []byte) (view.View, error) {
	return &Initiator{}, nil
}

// SlowInitiatorViewFactory returns an Initiator whose responder takes SlowResponderDelay.
type SlowInitiatorViewFactory struct{}

func (*SlowInitiatorViewFactory) NewView(_ []byte) (view.View, error) {
	return &Initiator{Slow: true}, nil
}
