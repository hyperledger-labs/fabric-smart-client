/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

import (
	"context"

	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type MSP struct {
	Name         string `yaml:"name,omitempty"`
	Type         string `yaml:"type,omitempty"`
	EnrollmentID string
	GetIdentity  driver.GetIdentityFunc
}

type Deserializer = driver2.SigDeserializer

//go:generate counterfeiter -o mock/config.go -fake-name Config . Config
type Config interface {
	NetworkName() string
	DefaultMSP() string
	MSPs() ([]config.MSP, error)
	TranslatePath(path string) string
}

//go:generate counterfeiter -o mock/signer_service.go -fake-name SignerService . SignerService
type SignerService interface {
	RegisterSigner(ctx context.Context, identity view.Identity, signer driver.Signer, verifier driver.Verifier) error
	IsMe(ctx context.Context, id view.Identity) bool
}

//go:generate counterfeiter -o mock/binder_service.go -fake-name BinderService . BinderService
type BinderService interface {
	Bind(ctx context.Context, longTerm view.Identity, ephemeral ...view.Identity) error
	GetIdentity(label string, pkiID []byte) (view.Identity, error)
}

//go:generate counterfeiter -o mock/config_provider.go -fake-name ConfigProvider . ConfigProvider
type ConfigProvider interface {
	driver.ConfigService
}

type DeserializerManager interface {
	AddDeserializer(deserializer Deserializer)
}

//go:generate counterfeiter -o mock/manager.go -fake-name Manager . Manager
type Manager interface {
	AddDeserializer(deserializer Deserializer)
	AddMSP(name, mspType, enrollmentID string, idGetter driver.GetIdentityFunc) error
	Config() Config
	DefaultMSP() string
	SignerService() SignerService
	CacheSize() int
	SetDefaultIdentity(id string, defaultIdentity view.Identity, defaultSigningIdentity SigningIdentity)
}

type IdentityLoader interface {
	Load(manager Manager, config config.MSP) error
}

// Identity refers to the creator of a tx;
type Identity interface {
	Serialize() ([]byte, error)

	Verify(msg, sig []byte) error
}

// SigningIdentity defines the functions necessary to sign an
// array of bytes; it is needed to sign the commands transmitted to
// the prover peer service.
//
//go:generate counterfeiter -o mock/signing_identity.go -fake-name SigningIdentity . SigningIdentity
type SigningIdentity interface {
	Identity // extends Identity

	Sign(msg []byte) ([]byte, error)
}
