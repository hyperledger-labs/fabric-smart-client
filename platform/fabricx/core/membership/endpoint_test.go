package membership

/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEndpointParser(t *testing.T) {
	tests := []struct {
		endpoint         string
		expectedEndpoint *endpoint
		expectedError    error
	}{
		{"id=0,broadcast,127.0.0.1:7050", &endpoint{ID: 0, Type: OrdererBroadcastType, Endpoint: "127.0.0.1:7050"}, nil},
		{"id=0,deliver,127.0.0.1:7050", &endpoint{ID: 0, Type: OrdererDeliverType, Endpoint: "127.0.0.1:7050"}, nil},
		{"id=100,deliver,host.docker.local:", &endpoint{ID: 100, Type: OrdererDeliverType, Endpoint: "host.docker.local:"}, nil},
		{"id=x,deliver,localhost:9999", nil, ErrInvalidEndpointFormat},
		{"id=0,unknown,127.0.0.1:7050", nil, ErrInvalidEndpointFormat},
		{"127.0.0.1:7050", nil, ErrInvalidEndpointFormat},
	}

	for i, tt := range tests {
		t.Run(fmt.Sprintf("testcase %d", i), func(t *testing.T) {
			ep, err := parseEndpoint(tt.endpoint)

			if tt.expectedError != nil {
				require.EqualError(t, err, tt.expectedError.Error())
			} else {
				require.NoError(t, err)
			}

			require.EqualValues(t, tt.expectedEndpoint, ep)
		})
	}
}
