/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package cache

import (
	pmsp "github.com/hyperledger/fabric-protos-go-apiv2/msp"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/cache"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/msp"
)

const (
	deserializeIdentityCacheSize = 100
	validateIdentityCacheSize    = 100
	satisfiesPrincipalCacheSize  = 100
)

var mspLogger = logging.MustGetLogger("msp")

func New(o msp.MSP) (msp.MSP, error) {
	mspLogger.Debugf("Creating Cache-MSP instance")
	if o == nil {
		return nil, errors.Errorf("invalid passed MSP. It must be different from nil")
	}

	theMsp := &cachedMSP{MSP: o}
	theMsp.cleanCache()

	return theMsp, nil
}

type cachedMSP struct {
	msp.MSP

	// cache for DeserializeIdentity.
	deserializeIdentityCache cache.Map[string, msp.Identity]

	// cache for validateIdentity. Only successful validations are cached, so
	// the presence of the key is the answer and the value is always true.
	validateIdentityCache cache.Map[string, bool]

	// basically a map of principals=>identities=>stringified to the outcome of
	// the check. A cached nil error is a cached success, so a hit is told from
	// a miss by the second return value rather than by the error being nil.
	satisfiesPrincipalCache cache.Map[string, error]
}

type cachedIdentity struct {
	msp.Identity
	cache *cachedMSP
}

func (id *cachedIdentity) SatisfiesPrincipal(principal *pmsp.MSPPrincipal) error {
	return id.cache.SatisfiesPrincipal(id.Identity, principal)
}

func (id *cachedIdentity) Validate() error {
	return id.cache.Validate(id.Identity)
}

type cachedSigningIdentity struct {
	cachedIdentity
	msp.SigningIdentity
}

func (id *cachedSigningIdentity) SatisfiesPrincipal(principal *pmsp.MSPPrincipal) error {
	return id.cachedIdentity.SatisfiesPrincipal(principal)
}

func (id *cachedSigningIdentity) Validate() error {
	return id.cachedIdentity.Validate()
}

func (id *cachedSigningIdentity) Sign(msg []byte) ([]byte, error) {
	return id.SigningIdentity.Sign(msg)
}

func (id *cachedSigningIdentity) GetPublicVersion() msp.Identity {
	return id.cache.wrap(id.SigningIdentity.GetPublicVersion())
}

func (c *cachedMSP) DeserializeIdentity(serializedIdentity []byte) (msp.Identity, error) {
	if id, ok := c.deserializeIdentityCache.Get(string(serializedIdentity)); ok {
		return c.wrap(id), nil
	}

	id, err := c.MSP.DeserializeIdentity(serializedIdentity)
	if err != nil {
		return nil, err
	}
	c.deserializeIdentityCache.Put(string(serializedIdentity), id)
	return c.wrap(id), nil
}

// wrap returns a cached identity wrapper that preserves the SigningIdentity
// interface if the underlying identity supports signing operations.
func (c *cachedMSP) wrap(id msp.Identity) msp.Identity {
	if sid, ok := id.(msp.SigningIdentity); ok {
		return &cachedSigningIdentity{
			Identity:        id,
			cache:           c,
			SigningIdentity: sid,
		}
	}
	return &cachedIdentity{
		Identity: id,
		cache:    c,
	}
}

func (c *cachedMSP) Setup(config *pmsp.MSPConfig) error {
	c.cleanCache()
	return c.MSP.Setup(config)
}

func (c *cachedMSP) Validate(id msp.Identity) error {
	identifier := id.GetIdentifier()
	key := identifier.Mspid + ":" + identifier.Id

	if _, ok := c.validateIdentityCache.Get(key); ok {
		// cache only stores if the identity is valid.
		return nil
	}

	err := c.MSP.Validate(id)
	if err == nil {
		c.validateIdentityCache.Put(key, true)
	}

	return err
}

func (c *cachedMSP) SatisfiesPrincipal(id msp.Identity, principal *pmsp.MSPPrincipal) error {
	identifier := id.GetIdentifier()
	identityKey := identifier.Mspid + ":" + identifier.Id
	principalKey := string(principal.PrincipalClassification) + string(principal.Principal)
	key := identityKey + principalKey

	if cachedErr, ok := c.satisfiesPrincipalCache.Get(key); ok {
		return cachedErr
	}

	err := c.MSP.SatisfiesPrincipal(id, principal)

	c.satisfiesPrincipalCache.Put(key, err)
	return err
}

func (c *cachedMSP) cleanCache() {
	c.deserializeIdentityCache = cache.NewSecondChanceCache[string, msp.Identity](
		deserializeIdentityCacheSize, 1, nil)
	c.satisfiesPrincipalCache = cache.NewSecondChanceCache[string, error](
		satisfiesPrincipalCacheSize, 1, nil)
	c.validateIdentityCache = cache.NewSecondChanceCache[string, bool](
		validateIdentityCacheSize, 1, nil)
}
