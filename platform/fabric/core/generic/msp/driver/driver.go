/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/config"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/core/sig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type MSP struct {
	Name         string `yaml:"name,omitempty"`
	Type         string `yaml:"type,omitempty"`
	EnrollmentID string
	GetIdentity  fdriver.GetIdentityFunc
}

type Config interface {
	NetworkName() string
	DefaultMSP() string
	MSPs() ([]config.MSP, error)
	TranslatePath(path string) string
}

type SignerService interface {
	RegisterSigner(identity view.Identity, signer fdriver.Signer, verifier fdriver.Verifier) error
}

type BinderService interface {
	Bind(longTerm view.Identity, ephemeral view.Identity) error
}

type ConfigProvider interface {
	driver.ConfigService
}

type DeserializerManager interface {
	AddDeserializer(deserializer sig.Deserializer)
}

type Manager interface {
	AddDeserializer(deserializer sig.Deserializer)
	AddMSP(name string, mspType string, enrollmentID string, idGetter fdriver.GetIdentityFunc)
	Config() Config
	DefaultMSP() string
	SignerService() SignerService
	ServiceProvider() view2.ServiceProvider
	CacheSize() int
	SetDefaultIdentity(id string, defaultIdentity view.Identity, defaultSigningIdentity SigningIdentity)
}

type IdentityLoader interface {
	Load(manager Manager, config config.MSP) error
}

// Identity refers to the creator of a tx;
type Identity interface {
	Serialize() ([]byte, error)

	Verify(msg []byte, sig []byte) error
}

// SigningIdentity defines the functions necessary to sign an
// array of bytes; it is needed to sign the commands transmitted to
// the prover peer service.
type SigningIdentity interface {
	Identity //extends Identity

	Sign(msg []byte) ([]byte, error)
}
