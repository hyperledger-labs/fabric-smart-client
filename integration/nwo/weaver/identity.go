/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package weaver

// {"credentials":{"certificate":"-----BEGIN CERTIFICATE-----\nMIICkDCCAjegAwIBAgIUOpqn9H6PU36pShxndsIuTuoCFKswCgYIKoZIzj0EAwIw\ncjELMAkGA1UEBhMCVVMxFzAVBgNVBAgTDk5vcnRoIENhcm9saW5hMQ8wDQYDVQQH\nEwZEdXJoYW0xGjAYBgNVBAoTEW9yZzEubmV0d29yazEuY29tMR0wGwYDVQQDExRj\nYS5vcmcxLm5ldHdvcmsxLmNvbTAeFw0yMDA4MTEwNjI5MDBaFw0yMTA4MTEwNjM0\nMDBaMEIxMDANBgNVBAsTBmNsaWVudDALBgNVBAsTBG9yZzEwEgYDVQQLEwtkZXBh\ncnRtZW50MTEOMAwGA1UEAxMFcmVsYXkwWTATBgcqhkjOPQIBBggqhkjOPQMBBwNC\nAAR2PO870eeQI0ZOmBNpK5nj+1Q+lUkRveOIlV7YQ8ZT5R7UJuC7fSBklxQKVvnr\nKGj8B5iWMJ3nQW6hcwhOkIEJo4HaMIHXMA4GA1UdDwEB/wQEAwIHgDAMBgNVHRMB\nAf8EAjAAMB0GA1UdDgQWBBT8vkQ+qstam5E0BqjRpjeHBt6AfTAfBgNVHSMEGDAW\ngBTWD60+eCHbDyD33PWbCxVuQqMAqTB3BggqAwQFBgcIAQRreyJhdHRycyI6eyJo\nZi5BZmZpbGlhdGlvbiI6Im9yZzEuZGVwYXJ0bWVudDEiLCJoZi5FbnJvbGxtZW50\nSUQiOiJyZWxheSIsImhmLlR5cGUiOiJjbGllbnQiLCJyZWxheSI6InRydWUifX0w\nCgYIKoZIzj0EAwIDRwAwRAIgXKrl7VBQwCqyVzWaerCHJZ0avzLnV4+sNfWDX3Ua\n3b0CIHvBEHAo0A5poBFCrYvJy2J7ql8JULE88dzUMCa/QlQJ\n-----END CERTIFICATE-----\n","privateKey":"-----BEGIN PRIVATE KEY-----\r\nMIGHAgEAMBMGByqGSM49AgEGCCqGSM49AwEHBG0wawIBAQQgSikOHkFWvw4057jO\r\nXAbpzvx7W+kOaCzK4qWZqO6IATuhRANCAAR2PO870eeQI0ZOmBNpK5nj+1Q+lUkR\r\nveOIlV7YQ8ZT5R7UJuC7fSBklxQKVvnrKGj8B5iWMJ3nQW6hcwhOkIEJ\r\n-----END PRIVATE KEY-----\r\n"},"mspId":"Org1MSP","type":"X.509","version":1}

type Credentials struct {
	Certificate string `json:"certificate"`
	PrivateKey  string `json:"privateKey"`
}

type Identity struct {
	Credentials Credentials `json:"credentials"`
	MspId       string      `json:"mspId"`
	Type        string      `json:"type"`
	Version     int         `json:"version"`
}
