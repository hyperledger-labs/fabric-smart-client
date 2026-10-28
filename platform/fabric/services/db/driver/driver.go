/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	dbdriver "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
)

type NamedDriver = driver.NamedDriver[Driver]

type EndorseTxStore = driver.EndorseTxStore[string]

type MetadataStore = driver.MetadataStore[string, []byte]

type EnvelopeStore = driver.EnvelopeStore[string]

type VaultStore = driver.VaultStore

type Driver interface {
	// NewEndorseTx returns a new EndorseTxStore for the passed data source and config
	NewEndorseTx(dbdriver.PersistenceName, ...string) (EndorseTxStore, error)
	// NewMetadata returns a new MetadataStore for the passed data source and config
	NewMetadata(dbdriver.PersistenceName, ...string) (MetadataStore, error)
	// NewEnvelope returns a new EnvelopeStore for the passed data source and config
	NewEnvelope(dbdriver.PersistenceName, ...string) (EnvelopeStore, error)
	// NewVault returns a new VaultStore for the passed data source and config
	NewVault(dbdriver.PersistenceName, ...string) (driver.VaultStore, error)
}
