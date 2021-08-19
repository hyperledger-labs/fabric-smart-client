/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package weaver

type Timeout struct {
	Peer map[string]string `json:"peer"`
}

type Connection struct {
	Timeout Timeout `json:"timeout"`
}

type Client struct {
	Organization string     `json:"organization"`
	Connection   Connection `json:"connection"`
}

type Organization struct {
	MSPID                  string   `json:"mspid"`
	Peers                  []string `json:"peers"`
	CertificateAuthorities []string `json:"certificateAuthorities"`
}

type Peer struct {
	URL         string                 `json:"url"`
	TLSCACerts  map[string]interface{} `json:"tlsCACerts"`
	GrpcOptions map[string]interface{} `json:"grpcOptions"`
}

type HttpOptions struct {
	Verify bool `json:"verify"`
}

type CertificationAuthority struct {
	Url         string            `json:"url"`
	CaName      string            `json:"caName"`
	TLSCACerts  map[string]string `json:"tlsCACerts"`
	HttpOptions HttpOptions       `json:"httpOptions"`
}

type ConnectionProfile struct {
	Name                   string                            `json:"name"`
	Version                string                            `json:"version"`
	Client                 Client                            `json:"client"`
	Organizations          map[string]Organization           `json:"organizations"`
	Peers                  map[string]Peer                   `json:"peers"`
	CertificateAuthorities map[string]CertificationAuthority `json:"certificateAuthorities"`
}
