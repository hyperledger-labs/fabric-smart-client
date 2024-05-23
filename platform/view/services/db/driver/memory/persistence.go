/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package mem

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/notifier"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/unversioned"
)

func NewVersionedPersistence() driver.VersionedPersistence {
	return New()
}

func NewVersionedPersistenceNotifier(dataSourceName string, config driver.Config) driver.VersionedNotifier {
	return notifier.NewVersioned(NewVersionedPersistence())
}

func NewUnversionedPersistence() driver.UnversionedPersistence {
	return &unversioned.Unversioned{Versioned: NewVersionedPersistence()}
}

func NewUnversionedPersistenceNotifier() driver.UnversionedNotifier {
	return notifier.NewUnversioned(NewUnversionedPersistence())
}
