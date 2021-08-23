/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package weaver

type (
	Admin struct {
		Name   string `json:"name"`
		Secret string `json:"secret"`
	}

	Attr struct {
		Name  string `json:"name"`
		Value string `json:"value"`
		Ecert bool   `json:"ecert"`
	}

	Relay struct {
		Name        string `json:"name"`
		Affiliation string `json:"affiliation"`
		Role        string `json:"role"`
		Attrs       []Attr `json:"attrs"`
	}

	Config struct {
		Admin Admin  `json:"admin"`
		Relay Relay  `json:"relay"`
		MspId string `json:"mspId"`
		CaUrl string `json:"caUrl"`
	}
)
