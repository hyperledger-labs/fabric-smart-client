/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package signedpingpong

import "github.com/hyperledger-labs/fabric-smart-client/platform/view/view"

// AliceInitiatorViewFactory creates AliceInitiator views.
type AliceInitiatorViewFactory struct{}

func (*AliceInitiatorViewFactory) NewView(_ []byte) (view.View, error) {
	return &AliceInitiator{}, nil
}
