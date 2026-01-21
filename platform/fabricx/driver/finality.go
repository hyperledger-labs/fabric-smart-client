/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/finality"
)

const (
	Valid   = fdriver.Valid
	Invalid = fdriver.Invalid
	Unknown = fdriver.Unknown
	Busy    = fdriver.Busy
)

type (
	TxID                    = driver.TxID
	FinalityListener        = fabric.FinalityListener
	FinalityListenerManager = finality.ListenerManager
)
