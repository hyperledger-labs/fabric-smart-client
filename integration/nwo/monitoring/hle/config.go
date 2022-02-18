/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package hle

type (
	Network struct {
		Name                 string `json:"name"`
		Profile              string `json:"profile"`
		EnableAuthentication bool   `json:"enableAuthentication"`
	}

	Config struct {
		NetworkConfigs map[string]Network `json:"network-configs"`
		License        string             `json:"license"`
	}
)
