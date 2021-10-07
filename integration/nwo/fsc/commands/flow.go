/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package commands

type Flow struct {
	NetworkPrefix string
	UserCert      string
	UserKey       string
	MSPID         string
	Server        string
	Function      string
	ClientCert    string
	ClientKey     string
	TLSCA         string
	Input         string
}

func (f Flow) SessionName() string {
	return f.NetworkPrefix + "-flow"
}

func (f Flow) Args() []string {
	args := []string{
		"--userCert", f.UserCert,
		"--userKey", f.UserKey,
		"--MSP", f.MSPID,
		"flow",
		"--endpoint", f.Server,
		"--function", f.Function,
		"--input", f.Input,
		"--peerTLSCA", f.TLSCA,
	}
	if f.ClientCert != "" {
		args = append(args, "--tlsCert", f.ClientCert)
	}
	if f.ClientKey != "" {
		args = append(args, "--tlsKey", f.ClientKey)
	}
	return args
}
