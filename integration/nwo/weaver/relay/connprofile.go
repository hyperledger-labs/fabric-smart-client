/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package relay

type Client struct {
	Organization string `json:"organization"`
}

type Organization struct {
	MSPID                  string   `json:"mspid"`
	Peers                  []string `json:"peers"`
	CertificateAuthorities []string `json:"certificateAuthorities"`
}

type Peer struct {
	URL        string            `json:"url"`
	TLSCACerts map[string]string `json:"tlsCACerts"`
}

type ConnectionProfile struct {
	Name          string                  `json:"name"`
	Version       string                  `json:"version"`
	Client        Client                  `json:"client"`
	Organizations map[string]Organization `json:"organizations"`
	Peers         map[string]Peer         `json:"peers"`
}
