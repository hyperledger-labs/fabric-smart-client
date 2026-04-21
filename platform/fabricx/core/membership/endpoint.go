/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package membership

import (
	"regexp"
	"strconv"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
)

const (
	OrdererBroadcastType = "broadcast"
	OrdererDeliverType   = "deliver"
)

var (
	endpointRegex            = regexp.MustCompile(`id=(\d+),(` + OrdererBroadcastType + `|` + OrdererDeliverType + `),(.*)`)
	ErrInvalidEndpointFormat = errors.New("invalid endpoint format")
)

type endpoint struct {
	ID       int
	Type     string
	Endpoint string
}

// parseEndpoint parses the fabric-x channel-config endpoint format
// `id=<group>,(broadcast|deliver),<host>:<port>` as emitted by the official
// fabric-x ansible collection and fabric-x-tool config-builder. A plain
// `<host>:<port>` value is accepted and treated as broadcast with ID=0 for
// backward compatibility.
func parseEndpoint(str string) (*endpoint, error) {
	match := endpointRegex.FindStringSubmatch(str)
	if len(match) == 4 {
		id, err := strconv.Atoi(match[1])
		if err != nil {
			return nil, errors.Wrap(err, "invalid endpoint id")
		}
		return &endpoint{ID: id, Type: match[2], Endpoint: match[3]}, nil
	}
	// Fallback: plain host:port. Treat as broadcast.
	if str != "" {
		return &endpoint{ID: 0, Type: OrdererBroadcastType, Endpoint: str}, nil
	}
	return nil, ErrInvalidEndpointFormat
}
