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
	myExp                    = regexp.MustCompile(`id=(\d+),(` + OrdererBroadcastType + `|` + OrdererDeliverType + `),(.*)`)
	ErrInvalidEndpointFormat = errors.New("invalid endpoint format")
)

type endpoint struct {
	ID       int
	Type     string
	Endpoint string
}

func parseEndpoint(str string) (*endpoint, error) {
	match := myExp.FindStringSubmatch(str)

	if len(match) != 4 {
		return nil, ErrInvalidEndpointFormat
	}

	id, err := strconv.Atoi(match[1])
	if err != nil {
		return nil, errors.Wrap(err, "invalid endpoint id")
	}

	return &endpoint{
		ID:       id,
		Type:     match[2],
		Endpoint: match[3],
	}, nil
}
