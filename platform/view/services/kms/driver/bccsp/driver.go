/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

import (
	kms "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/kms"
)


type Driver struct{}

func (d *Driver)Load(identity string) {

}

func init() {
	kms.Register("bccsp", &Driver{})
}