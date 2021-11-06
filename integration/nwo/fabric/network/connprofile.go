/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package network

type T struct {
}

type Timeout struct {
	Peer map[string]string `json:"peer"`
}

type Connection struct {
	Timeout Timeout `json:"timeout"`
}

type AdminCredential struct {
	Id       string `json:"id"`
	Password string `json:"password"`
}

type Client struct {
	AdminCredential      AdminCredential `json:"adminCredential"`
	Organization         string          `json:"organization"`
	EnableAuthentication bool            `json:"enableAuthentication"`
	TlsEnable            bool            `json:"tlsEnable"`
	Connection           Connection      `json:"connection"`
}

type Organization struct {
	MSPID                  string                 `json:"mspid"`
	Peers                  []string               `json:"peers"`
	CertificateAuthorities []string               `json:"certificateAuthorities,omitempty"`
	SignedCert             map[string]interface{} `json:"signedCert,omitempty"`
	AdminPrivateKey        map[string]interface{} `json:"adminPrivateKey,omitempty"`
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
	Orderers []string               `json:"orderers"`
	Peers    map[string]ChannelPeer `json:"peers"`
}

type Orderer struct {
	URL         string                 `json:"url"`
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
