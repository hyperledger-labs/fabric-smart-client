/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package channelconfig

import (
	"fmt"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/membership/channelconfig/capabilities"
	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
	ab "github.com/hyperledger/fabric-protos-go-apiv2/orderer"
)

const (
	// OrdererGroupKey is the group name for the orderer config.
	OrdererGroupKey = "Orderer"
)

const (
	// ConsensusTypeKey is the cb.ConfigItem type key name for the ConsensusType message.
	ConsensusTypeKey = "ConsensusType"

	// BatchSizeKey is the cb.ConfigItem type key name for the BatchSize message.
	BatchSizeKey = "BatchSize"

	// BatchTimeoutKey is the cb.ConfigItem type key name for the BatchTimeout message.
	BatchTimeoutKey = "BatchTimeout"

	// ChannelRestrictionsKey is the key name for the ChannelRestrictions message.
	ChannelRestrictionsKey = "ChannelRestrictions"

	// EndpointsKey is the cb.COnfigValue key name for the Endpoints message in the OrdererOrgGroup.
	EndpointsKey = "Endpoints"
)

// OrdererProtos is used as the source of the OrdererConfig.
type OrdererProtos struct {
	ConsensusType       *ab.ConsensusType
	BatchSize           *ab.BatchSize
	BatchTimeout        *ab.BatchTimeout
	ChannelRestrictions *ab.ChannelRestrictions
	Orderers            *cb.Orderers
	Capabilities        *cb.Capabilities
}

// OrdererConfig holds the orderer configuration information.
type OrdererConfig struct {
	protos *OrdererProtos
	orgs   map[string]OrdererOrg

	batchTimeout time.Duration
}

// OrdererOrgProtos are deserialized from the Orderer org config values
type OrdererOrgProtos struct {
	Endpoints *cb.OrdererAddresses
}

// OrdererOrgConfig defines the configuration for an orderer org
type OrdererOrgConfig struct {
	*OrganizationConfig
	protos *OrdererOrgProtos
	name   string
}

// Endpoints returns the set of addresses this ordering org exposes as orderers
func (oc *OrdererOrgConfig) Endpoints() []string {
	if oc.protos == nil || oc.protos.Endpoints == nil {
		return nil
	}

	return oc.protos.Endpoints.Addresses
}

// NewOrdererOrgConfig returns an orderer org config built from the given ConfigGroup.
func NewOrdererOrgConfig(orgName string, orgGroup *cb.ConfigGroup, mspConfigHandler *MSPConfigHandler, channelCapabilities ChannelCapabilities) (*OrdererOrgConfig, error) {
	if len(orgGroup.Groups) > 0 {
		return nil, fmt.Errorf("OrdererOrg config does not allow sub-groups")
	}

	if !channelCapabilities.OrgSpecificOrdererEndpoints() {
		if _, ok := orgGroup.Values[EndpointsKey]; ok {
			return nil, errors.Errorf("Orderer Org %s cannot contain endpoints value until V1_4_2+ capabilities have been enabled", orgName)
		}
	}

	protos := &OrdererOrgProtos{}
	orgProtos := &OrganizationProtos{}

	if err := DeserializeProtoValuesFromGroup(orgGroup, protos, orgProtos); err != nil {
		return nil, errors.Wrap(err, "failed to deserialize values")
	}

	ooc := &OrdererOrgConfig{
		name:   orgName,
		protos: protos,
		OrganizationConfig: &OrganizationConfig{
			name:             orgName,
			protos:           orgProtos,
			mspConfigHandler: mspConfigHandler,
		},
	}

	if err := ooc.Validate(); err != nil {
		return nil, err
	}

	return ooc, nil
}

func (ooc *OrdererOrgConfig) Validate() error {
	return ooc.OrganizationConfig.Validate()
}

// NewOrdererConfig creates a new instance of the orderer config.
func NewOrdererConfig(ordererGroup *cb.ConfigGroup, mspConfig *MSPConfigHandler, channelCapabilities ChannelCapabilities) (*OrdererConfig, error) {
	oc := &OrdererConfig{
		protos: &OrdererProtos{},
		orgs:   make(map[string]OrdererOrg),
	}

	if err := DeserializeProtoValuesFromGroup(ordererGroup, oc.protos); err != nil {
		return nil, errors.Wrap(err, "failed to deserialize values")
	}

	if err := oc.Validate(); err != nil {
		return nil, err
	}

	for orgName, orgGroup := range ordererGroup.Groups {
		var err error
		if oc.orgs[orgName], err = NewOrdererOrgConfig(orgName, orgGroup, mspConfig, channelCapabilities); err != nil {
			return nil, err
		}
	}

	if channelCapabilities.ConsensusTypeBFT() {
		if err := oc.validateAllOrgsHaveEndpoints(); err != nil {
			return nil, err
		}
	}

	return oc, nil
}

// ConsensusType returns the configured consensus type.
func (oc *OrdererConfig) ConsensusType() string {
	return oc.protos.ConsensusType.Type
}

// ConsensusMetadata returns the metadata associated with the consensus type.
func (oc *OrdererConfig) ConsensusMetadata() []byte {
	return oc.protos.ConsensusType.Metadata
}

// ConsensusState return the consensus type state.
func (oc *OrdererConfig) ConsensusState() ab.ConsensusType_State {
	return oc.protos.ConsensusType.State
}

// BatchSize returns the maximum number of messages to include in a block.
func (oc *OrdererConfig) BatchSize() *ab.BatchSize {
	return oc.protos.BatchSize
}

// BatchTimeout returns the amount of time to wait before creating a batch.
func (oc *OrdererConfig) BatchTimeout() time.Duration {
	return oc.batchTimeout
}

// MaxChannelsCount returns the maximum count of channels this orderer supports.
func (oc *OrdererConfig) MaxChannelsCount() uint64 {
	return oc.protos.ChannelRestrictions.MaxCount
}

// Organizations returns a map of the orgs in the channel.
func (oc *OrdererConfig) Organizations() map[string]OrdererOrg {
	return oc.orgs
}

func (oc *OrdererConfig) Consenters() []*cb.Consenter {
	return oc.protos.Orderers.ConsenterMapping
}

// Capabilities returns the capabilities the ordering network has for this channel.
func (oc *OrdererConfig) Capabilities() OrdererCapabilities {
	return capabilities.NewOrdererProvider(oc.protos.Capabilities.Capabilities)
}

func (oc *OrdererConfig) Validate() error {
	for _, validator := range []func() error{
		oc.validateBatchSize,
		oc.validateBatchTimeout,
	} {
		if err := validator(); err != nil {
			return err
		}
	}

	return nil
}

func (oc *OrdererConfig) validateBatchSize() error {
	if oc.protos.BatchSize.MaxMessageCount == 0 {
		return fmt.Errorf("attempted to set the batch size max message count to an invalid value: 0")
	}
	if oc.protos.BatchSize.AbsoluteMaxBytes == 0 {
		return fmt.Errorf("attempted to set the batch size absolute max bytes to an invalid value: 0")
	}
	if oc.protos.BatchSize.PreferredMaxBytes == 0 {
		return fmt.Errorf("attempted to set the batch size preferred max bytes to an invalid value: 0")
	}
	if oc.protos.BatchSize.PreferredMaxBytes > oc.protos.BatchSize.AbsoluteMaxBytes {
		return fmt.Errorf("attempted to set the batch size preferred max bytes (%v) greater than the absolute max bytes (%v)", oc.protos.BatchSize.PreferredMaxBytes, oc.protos.BatchSize.AbsoluteMaxBytes)
	}
	return nil
}

func (oc *OrdererConfig) validateBatchTimeout() error {
	var err error
	oc.batchTimeout, err = time.ParseDuration(oc.protos.BatchTimeout.Timeout)
	if err != nil {
		return fmt.Errorf("attempted to set the batch timeout to a invalid value: %s", err)
	}
	if oc.batchTimeout <= 0 {
		return fmt.Errorf("attempted to set the batch timeout to a non-positive value: %s", oc.batchTimeout)
	}
	return nil
}

func (oc *OrdererConfig) validateAllOrgsHaveEndpoints() error {
	var orgsMissingEndpoints []string

	for _, org := range oc.Organizations() {
		if len(org.Endpoints()) == 0 {
			orgsMissingEndpoints = append(orgsMissingEndpoints, org.Name())
		}
	}

	if len(orgsMissingEndpoints) > 0 {
		return errors.Errorf("some orderer organizations endpoints are empty: %s", orgsMissingEndpoints)
	}

	return nil
}
