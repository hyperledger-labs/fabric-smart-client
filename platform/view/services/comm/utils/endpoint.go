/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package utils

import (
	"fmt"
	"net"
	"strings"

	"github.com/pkg/errors"
)

func AddressToEndpoint(endpoint string) (string, error) {
	s := strings.Split(endpoint, ":")
	if len(s) > 2 {
		if host := strings.Join(s[:len(s)-1], ":"); host == "[::1]" {
			s = []string{"0.0.0.0", s[len(s)-1]}
		}
	}
	if len(s) != 2 {
		return "", errors.Errorf("invalid endpoint [%s], expected 2 components, got [%d]", endpoint, len(s))
	}
	var addrS string
	addr, err := net.LookupIP(s[0])
	if err != nil {
		addrS = s[0]
	} else {
		addrS = addr[0].String()
	}
	port := s[1]

	return fmt.Sprintf("/ip4/%s/tcp/%s", addrS, port), nil
}
