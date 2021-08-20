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

type ChannelPeer struct {
	EndorsingPeer  bool `json:"endorsingPeer"`
	ChaincodeQuery bool `json:"chaincodeQuery"`
	LedgerQuery    bool `json:"ledgerQuery"`
	EventSource    bool `json:"eventSource"`
	Discover       bool `json:"discover"`
}

type Channel struct {
	Orderers []string                 `json:"orderers"`
	Peers    []map[string]ChannelPeer `json:"peers"`
}

type Orderer struct {
	Url         string                 `json:"url"`
	TLSCACerts  map[string]interface{} `json:"tlsCACerts"`
	GrpcOptions map[string]interface{} `json:"grpcOptions"`
}

type ConnectionProfile struct {
	Name                   string                            `json:"name"`
	Version                string                            `json:"version"`
	Client                 Client                            `json:"client"`
	Channels               map[string]Channel                `json:"channels"`
	Orderers               map[string]Orderer                `json:"orderers"`
	Organizations          map[string]Organization           `json:"organizations"`
	Peers                  map[string]Peer                   `json:"peers"`
	CertificateAuthorities map[string]CertificationAuthority `json:"certificateAuthorities"`
}
