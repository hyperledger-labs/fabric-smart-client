/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package id

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/kms"
)

func NewKMSDriver(config driver.ConfigService) (*kms.KMS, error) {
	var idType string
	if idType = config.GetString("fsc.identity.type"); len(idType) == 0 {
		idType = "file"
	}
	return kms.Get(idType)
}
