/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package transaction

import (
	"testing"

	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
)

// TestUnpackEnvelopePayload_NilHeaderDoesNotPanic covers a payload with no Header field:
// UnpackEnvelopePayload must unmarshal the channel header via the nil-safe GetHeader()
// accessor instead of dereferencing a nil Header. A nil ChannelHeader decodes to the
// zero-value HeaderType (HeaderType_MESSAGE), so this is accepted rather than rejected.
func TestUnpackEnvelopePayload_NilHeaderDoesNotPanic(t *testing.T) {
	t.Parallel()

	payloadRaw, err := proto.Marshal(&common.Payload{Data: []byte("data")})
	require.NoError(t, err)

	var upe *UnpackedEnvelope
	require.NotPanics(t, func() {
		upe, _, err = UnpackEnvelopePayload(payloadRaw)
	})
	require.NoError(t, err)
	require.Equal(t, []byte("data"), upe.Results)
}
