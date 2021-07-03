/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package generic

import (
	pb "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/pkg/errors"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
)

func (c *channel) Status(txid string) (driver.ValidationCode, []string, error) {
	vc, err := c.vault.Status(txid)
	if err != nil {
		return driver.Unknown, nil, err
	}
	if c.externalCommitter == nil {
		return vc, nil, nil
	}

	_, dependantTxIDs, _, err := c.externalCommitter.Status(txid)
	if err != nil {
		return driver.Unknown, nil, err
	}
	if vc == driver.Unknown && len(dependantTxIDs) != 0 {
		return driver.HasDependencies, dependantTxIDs, nil
	}
	return vc, dependantTxIDs, nil
}

func (c *channel) ProcessNamespace(nss ...string) error {
	c.processNamespaces = append(c.processNamespaces, nss...)
	return nil
}

func (c *channel) GetProcessNamespace() []string {
	return c.processNamespaces
}

func (c *channel) DiscardTx(txid string) error {
	logger.Debugf("Discarding transaction [%s]", txid)

	vc, deps, err := c.Status(txid)
	if err != nil {
		return errors.WithMessagef(err, "failed getting tx's status in state db [%s]", txid)
	}
	if vc == driver.Unknown {
		return nil
	}

	c.vault.DiscardTx(txid)
	for _, dep := range deps {
		c.vault.DiscardTx(dep)
	}
	return nil
}

func (c *channel) CommitTX(txid string, block uint64, indexInBlock int, envelope []byte) (err error) {
	logger.Debugf("Committing transaction [%s,%d,%d]", txid, block, indexInBlock)
	defer logger.Debugf("Committing transaction [%s,%d,%d] done [%s]", txid, block, indexInBlock, err)

	vc, deps, err := c.Status(txid)
	if err != nil {
		return errors.WithMessagef(err, "failed getting tx's status in state db [%s]", txid)
	}

	switch vc {
	case driver.Valid:
		// This should generate a panic
		logger.Debugf("[%s] is already valid", txid)
		return errors.Errorf("[%s] is already valid", txid)
	case driver.Invalid:
		// This should generate a panic
		logger.Debugf("[%s] is invalid", txid)
		return errors.Errorf("[%s] is invalid", txid)
	case driver.Unknown:
		return c.commitUnknown(txid, block, indexInBlock)
	case driver.HasDependencies:
		return c.commitDeps(txid, block, indexInBlock)
	case driver.Busy:
		return c.commit(txid, deps, block, indexInBlock, envelope)
	default:
		return errors.Errorf("invalid status code [%d] for [%s]", vc, txid)
	}
}

func (c *channel) commitUnknown(txid string, block uint64, indexInBlock int) error {
	if len(c.processNamespaces) == 0 {
		// This should be ignored
		logger.Debugf("[%s] is unknown and will be ignored", txid)
		return nil
	}

	logger.Debugf("[%s] is unknown but will be processed for known namespaces", txid)
	pt, err := c.GetTransactionByID(txid)
	if err != nil {
		return errors.WithMessagef(err, "failed fetching tx [%s]", txid)
	}
	if !pt.IsValid() {
		return errors.Errorf("fetched tx [%s] should have been valid, instead it is [%s]", txid, pb.TxValidationCode_name[pt.ValidationCode()])
	}

	rws, err := c.GetRWSet(txid, pt.Results())
	if err != nil {
		return errors.WithMessagef(err, "failed getting rwset for tx [%s]", txid)
	}
	found := false
	logger.Debugf("[%s] contains namespaces [%v]", txid, rws.Namespaces())
	for _, ns := range rws.Namespaces() {
		for _, pns := range c.processNamespaces {
			if ns == pns {
				found = true
				break
			}
		}
		if found {
			break
		}
	}
	rws.Done()

	if !found {
		logger.Debugf("[%s] no known namespaces found", txid)
		// nothing to commit
		return nil
	}

	// commit this transaction because it contains one of ne namespace to be processed anyway
	logger.Debugf("[%s] known namespaces found, commit", txid)
	return c.commit(txid, nil, block, indexInBlock, nil)
}

func (c *channel) commit(txid string, deps []string, block uint64, indexInBlock int, envelope []byte) error {
	logger.Debugf("[%s] is known.", txid)

	switch {
	case len(deps) != 0:
		if err := c.commitExternal(txid, block, indexInBlock); err != nil {
			return err
		}
	default:
		if err := c.commitLocal(txid, block, indexInBlock, envelope); err != nil {
			return err
		}
	}
	return nil
}

func (c *channel) commitDeps(txid string, block uint64, indexInBlock int) error {
	// This should not generate a panic if the transaction is deemed invalid
	logger.Debugf("[%s] is unknown but have dependencies, commit as multi-shard pvt", txid)

	// Validate and commit
	vc, err := c.externalCommitter.Validate(txid)
	if err != nil {
		return errors.WithMessagef(err, "failed validating transaction [%s]", txid)
	}
	switch vc {
	case driver.Valid:
		if err := c.externalCommitter.CommitTX(txid, block, indexInBlock); err != nil {
			return errors.WithMessagef(err, "failed committing tx [%s]", txid)
		}
		return nil
	case driver.Invalid:
		if err := c.externalCommitter.DiscardTX(txid); err != nil {
			logger.Errorf("failed committing tx [%s] with err [%s]", txid, err)
		}
		return nil
	}
	return nil
}

func (c *channel) commitExternal(txid string, block uint64, indexInBlock int) error {
	logger.Debugf("[%s] Committing as multi-shard pvt.", txid)

	// Ask for finality
	_, _, parties, err := c.externalCommitter.Status(txid)
	if err != nil {
		return errors.Wrapf(err, "failed getting parties for [%s]", txid)
	}
	if err := c.IsFinalForParties(txid, parties...); err != nil {
		return err
	}

	// Validate and commit
	vc, err := c.externalCommitter.Validate(txid)
	if err != nil {
		return errors.WithMessagef(err, "failed validating transaction [%s]", txid)
	}
	switch vc {
	case driver.Valid:
		if err := c.externalCommitter.CommitTX(txid, block, indexInBlock); err != nil {
			return errors.WithMessagef(err, "failed committing tx [%s]", txid)
		}
		return nil
	case driver.Invalid:
		if err := c.externalCommitter.DiscardTX(txid); err != nil {
			logger.Errorf("failed committing tx [%s] with err [%s]", txid, err)
		}
		return nil
	}
	return nil
}

func (c *channel) commitLocal(txid string, block uint64, indexInBlock int, envelope []byte) error {
	// This is a normal transaction, validated by Fabric.
	// Commit it cause Fabric says it is valid.
	logger.Debugf("[%s] Committing", txid)

	// Match rwsets if envelope is not empty
	if len(envelope) != 0 {
		pt, err := newProcessedTransactionFromEnvelopeRaw(envelope)
		if err != nil {
			return err
		}

		if err := c.vault.Match(txid, pt.Results()); err != nil {
			return err
		}
	}

	// Post-Processes
	logger.Debugf("[%s] Post Processes", txid)

	if err := c.postProcessTx(txid); err != nil {
		// This should generate a panic
		return err
	}

	// Commit
	logger.Debugf("[%s] Commit in vault", txid)
	if err := c.vault.CommitTX(txid, block, indexInBlock); err != nil {
		// This should generate a panic
		return err
	}

	return nil
}

func (c *channel) postProcessTx(txid string) error {
	if err := c.network.ProcessorManager().ProcessByID(c.name, txid); err != nil {
		// This should generate a panic
		return err
	}
	return nil
}
