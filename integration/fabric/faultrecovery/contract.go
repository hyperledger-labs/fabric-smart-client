/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package faultrecovery

import (
	"context"
	"os"
	"slices"

	"github.com/onsi/gomega"
	"go.yaml.in/yaml/v3"

	"github.com/hyperledger-labs/fabric-smart-client/integration"
	nwonode "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc/node"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/sdk/dig"
	sdk "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/sdk/dig"
	viewsdk "github.com/hyperledger-labs/fabric-smart-client/platform/view/sdk/dig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
	drivercommon "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common"
	sqlcommon "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common/migrate"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/postgres"
)

const (
	// ContractProbeTable is the table of the contract probe store, in the default
	// persistence under the default table prefix.
	ContractProbeTable = "fsc_contract_probe"
	// contractProbeStepsKey is the core.yaml key that sets how many of contractProbeSteps
	// a node's binary knows. A node without it does not open the store.
	contractProbeStepsKey = "faultrecovery.contractProbeSteps"
)

// contractProbeSteps are the steps of the contract probe store: step 1 creates the table
// with the value in old_v, step 2 adds new_v (expand), and step 3 moves every value to
// new_v and drops old_v (contract). A replica configured with contractProbeStepsKey: 1
// stands for the previous release and one with 3 for a release that brings a contract
// step, so a spec runs both releases on one database with a single binary: migrate sees
// a store only through its steps.
var contractProbeSteps = []migrate.Step{
	{Version: 1, Name: "create", SQL: []string{"CREATE TABLE " + ContractProbeTable + " (id INT PRIMARY KEY, old_v INT)"}},
	{Version: 2, Name: "add new_v", SQL: []string{"ALTER TABLE " + ContractProbeTable + " ADD COLUMN new_v INT"}},
	{Version: 3, Name: "drop old_v", Contract: true, SQL: []string{
		// Backfill first: replicas of the previous release write old_v until this step runs.
		"UPDATE " + ContractProbeTable + " SET new_v = old_v WHERE new_v IS NULL",
		"ALTER TABLE " + ContractProbeTable + " DROP COLUMN old_v",
	}},
}

// ContractProbeSDK is the Fabric SDK plus the contract probe store, which it opens on a
// node that sets contractProbeStepsKey: from `node migrate` through the store openers,
// and at start, so that the node records itself in the openers table and runs the
// probe's pending steps like every FSC store.
type ContractProbeSDK struct {
	dig.SDK
}

// NewContractProbeSDK returns a ContractProbeSDK on top of a fresh Fabric SDK for
// registry. nwo calls it from the generated main of each FSC node in the topology.
func NewContractProbeSDK(registry services.Registry) *ContractProbeSDK {
	return &ContractProbeSDK{SDK: sdk.NewSDK(registry)}
}

// Install installs the Fabric SDK and registers the probe with the store openers that
// `node migrate` calls.
func (p *ContractProbeSDK) Install() error {
	if err := p.SDK.Install(); err != nil {
		return err
	}
	return p.Container().Provide(func() viewsdk.StoreOpenerEntry {
		return viewsdk.StoreOpenerEntry{Opener: viewsdk.StoreOpenerFunc(func(context.Context) error {
			return p.Container().Invoke(openContractProbe)
		})}
	})
}

// Start starts the Fabric SDK and opens the probe.
func (p *ContractProbeSDK) Start(ctx context.Context) error {
	if err := p.SDK.Start(ctx); err != nil {
		return err
	}
	return p.Container().Invoke(openContractProbe)
}

// SetContractProbeSteps sets contractProbeStepsKey to steps in the core.yaml of the
// stopped FSC replica with the given unique name, such as bob.0, which reads it on its
// next start. Zero removes the key, so the replica no longer opens the probe.
func SetContractProbeSteps(ii *integration.Infrastructure, replica string, steps int) {
	i := slices.IndexFunc(ii.FscPlatform.Peers, func(r *nwonode.Replica) bool { return r.UniqueName == replica })
	gomega.ExpectWithOffset(1, i).NotTo(gomega.Equal(-1), "no FSC replica [%s]", replica)
	path := ii.FscPlatform.NodeConfigPath(ii.FscPlatform.Peers[i])
	raw, err := os.ReadFile(path)
	gomega.ExpectWithOffset(1, err).NotTo(gomega.HaveOccurred())
	cfg := map[string]any{}
	gomega.ExpectWithOffset(1, yaml.Unmarshal(raw, &cfg)).To(gomega.Succeed())
	section, _ := cfg["faultrecovery"].(map[string]any) // contractProbeStepsKey
	if section == nil {
		section = map[string]any{}
		cfg["faultrecovery"] = section
	}
	if steps == 0 {
		delete(section, "contractProbeSteps")
	} else {
		section["contractProbeSteps"] = steps
	}
	raw, err = yaml.Marshal(cfg)
	gomega.ExpectWithOffset(1, err).NotTo(gomega.HaveOccurred())
	gomega.ExpectWithOffset(1, os.WriteFile(path, raw, 0o600)).To(gomega.Succeed())
}

// contractProbe is a Versioned store with the first n of contractProbeSteps.
type contractProbe struct{ n int }

func (contractProbe) StoreName() string       { return ContractProbeTable }
func (p contractProbe) Steps() []migrate.Step { return contractProbeSteps[:p.n] }
func (contractProbe) CreateSchema() error {
	return errors.New("the contract probe is Versioned; migrate never calls CreateSchema")
}

// openContractProbe opens the contract probe store with the steps the node's
// contractProbeStepsKey sets, through the default Postgres persistence. It does nothing
// when the key is unset.
func openContractProbe(config driver.Config, dbProvider postgres.DbProvider) error {
	if !config.IsSet(contractProbeStepsKey) {
		return nil
	}
	var n int
	if err := config.UnmarshalKey(contractProbeStepsKey, &n); err != nil {
		return errors.Wrapf(err, "failed to read %s", contractProbeStepsKey)
	}
	if n < 1 || n > len(contractProbeSteps) {
		return errors.Errorf("%s must be between 1 and %d, got %d", contractProbeStepsKey, len(contractProbeSteps), n)
	}
	cp := postgres.NewConfigProvider(drivercommon.NewConfig(config))
	_, err := postgres.NewPersistenceWithOpts(cp, dbProvider, "", func(*drivercommon.RWDB, sqlcommon.TableNames) (contractProbe, error) {
		return contractProbe{n: n}, nil
	})
	return errors.WithMessage(err, "failed to open the contract probe store")
}
