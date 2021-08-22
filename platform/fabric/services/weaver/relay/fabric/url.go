/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabric

import (
	url2 "net/url"
	"strings"

	"github.com/pkg/errors"
)

type ID struct {
	Network   string
	Channel   string
	Chaincode string
}

// URLToID converts a Fabric url ('fabric://<network-id>.<channel-id>.<chaincode-id>/`) to its components
func URLToID(url string) (*ID, error) {
	u, err := url2.Parse(url)
	if err != nil {
		return nil, errors.Wrapf(err, "failed parsing url")
	}
	if u.Scheme != "fabric" {
		return nil, errors.Errorf("invalid scheme, expected fabric, got [%s]", u.Scheme)
	}

	res := strings.Split(u.Host, ".")
	if len(res) != 3 {
		return nil, errors.Errorf("invalid host, expected 3 components, found [%d,%v]", len(res), res)
	}
	return &ID{
		Network: res[0], Channel: res[1], Chaincode: res[2],
	}, nil
}
