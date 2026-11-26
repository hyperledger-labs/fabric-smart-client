/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// Package views holds the FSC views every tlog node runs: logging an entry,
// endorsing it, and cosigning checkpoints.
package views

import (
	"math"
	"reflect"
	"strconv"
	"sync"

	"golang.org/x/mod/sumdb/note"

	"github.com/hyperledger-labs/fabric-smart-client/examples/tlog/translog"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/committer/queryservice"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

const (
	// Namespace is where the log lives on the ledger.
	Namespace = "tlog"
	// SizeKey holds the number of entries in the log, in decimal.
	SizeKey = "size"
)

// EntryKey is the key of the entry at index i: its canonicalized body.
func EntryKey(i uint64) string {
	return "entry_" + strconv.FormatUint(i, 10)
}

// LogService is a node's view of the log: the Merkle tree it built from the
// ledger, and the keys and peers the init view handed it.
//
// ponytail: the tree lives in memory and is rebuilt from the ledger after a
// restart, which is O(n); persist the hashes in the node's KVS if restarts
// matter.
type LogService struct {
	mu        sync.Mutex
	tree      *translog.Tree
	signer    note.Signer
	verifiers []note.Verifier
	peers     []view.Identity
}

// Sync appends the entries committed on the ledger since the last Sync.
func (s *LogService) Sync(qs queryservice.QueryService) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	size, _, err := readSize(qs)
	if err != nil {
		return err
	}
	// Grow a copy, so a tree returned by Tree never changes under its reader.
	next := translog.Tree{}
	if s.tree != nil {
		next = *s.tree
	}
	if size <= next.Size() {
		return nil
	}
	keys := make([]driver.PKey, 0, size-next.Size())
	for i := next.Size(); i < size; i++ {
		keys = append(keys, EntryKey(i))
	}
	values, err := qs.GetStates(map[driver.Namespace][]driver.PKey{Namespace: keys})
	if err != nil {
		return errors.Wrap(err, "failed reading the log entries")
	}
	for _, key := range keys {
		v, ok := values[Namespace][key]
		if !ok || len(v.Raw) == 0 {
			return errors.Errorf("entry [%s] is missing", key)
		}
		if err := next.Append(v.Raw); err != nil {
			return err
		}
	}
	s.tree = &next
	return nil
}

// Tree returns the tree as of the last Sync. Sync never changes a tree it
// returned, so the caller may read it without locking.
func (s *LogService) Tree() *translog.Tree {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.tree == nil {
		return &translog.Tree{}
	}
	return s.tree
}

// Checkpoint syncs the tree and returns its head signed by all three orgs. It
// fails if a peer refuses to cosign.
func (s *LogService) Checkpoint(viewCtx view.Context) ([]byte, error) {
	cp, t, refusals, err := s.checkpointAt(viewCtx)
	if err != nil {
		return nil, err
	}
	if len(refusals) > 0 {
		return nil, errors.Errorf("peers refused to cosign size %d: %v", t.Size(), refusals)
	}
	return cp, nil
}

// checkpointAt syncs the tree and cosigns its head with the other two orgs,
// returning the checkpoint, the tree it was built against, and the reasons
// any peer refused, without treating a refusal as an error: Checkpoint uses
// this to fail on any refusal, CheckpointView to report them instead.
func (s *LogService) checkpointAt(viewCtx view.Context) (cp []byte, tree *translog.Tree, refusals []string, err error) {
	qs, err := queryServiceOf(viewCtx)
	if err != nil {
		return nil, nil, nil, err
	}
	if err := s.Sync(qs); err != nil {
		return nil, nil, nil, err
	}
	t := s.Tree()
	root, err := t.Root(t.Size())
	if err != nil {
		return nil, nil, nil, err
	}
	cp, refusals, err = s.Cosign(viewCtx, t.Size(), root)
	if err != nil {
		return nil, nil, nil, err
	}
	return cp, t, refusals, nil
}

// Cosign signs the checkpoint (size, root) and asks every peer to cosign it.
// It returns the note with every signature collected, and the reasons of the
// peers that refused.
func (s *LogService) Cosign(viewCtx view.Context, size uint64, root []byte) ([]byte, []string, error) {
	signer, verifiers, peers, err := s.keys()
	if err != nil {
		return nil, nil, err
	}
	own, err := translog.SignCheckpoint(size, root, signer)
	if err != nil {
		return nil, nil, err
	}
	notes := [][]byte{own}
	var refusals []string
	for _, peer := range peers {
		res, err := viewCtx.RunView(&CosignView{Peer: peer, Size: size, Root: root})
		if err != nil {
			return nil, nil, errors.WithMessagef(err, "failed asking [%s] to cosign", peer)
		}
		resp, ok := res.(*CosignResponse)
		if !ok {
			return nil, nil, errors.Errorf("unexpected cosign result type [%T]", res)
		}
		if resp.Refused != "" {
			refusals = append(refusals, resp.Refused)
			continue
		}
		notes = append(notes, resp.Note)
	}
	cp, err := translog.MergeSignatures(verifiers, notes...)
	if err != nil {
		return nil, nil, err
	}
	return cp, refusals, nil
}

// sign signs the checkpoint (size, root) with this node's key alone.
func (s *LogService) sign(size uint64, root []byte) ([]byte, error) {
	signer, _, _, err := s.keys()
	if err != nil {
		return nil, err
	}
	return translog.SignCheckpoint(size, root, signer)
}

// setup loads the keys and peers of setup, replacing any loaded before.
func (s *LogService) setup(signer note.Signer, verifiers []note.Verifier, peers []view.Identity) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.signer, s.verifiers, s.peers = signer, verifiers, peers
}

func (s *LogService) keys() (note.Signer, []note.Verifier, []view.Identity, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.signer == nil {
		return nil, nil, nil, errors.New("the node has no log key: call init first")
	}
	return s.signer, s.verifiers, s.peers, nil
}

// readSize returns the committed log size, 0 if nothing is logged yet, and
// the version of size, nil if nothing is logged yet.
func readSize(qs queryservice.QueryService) (uint64, driver.RawVersion, error) {
	v, err := qs.GetState(Namespace, SizeKey)
	if err != nil {
		return 0, nil, errors.Wrap(err, "failed reading the log size")
	}
	if v == nil {
		return 0, nil, nil
	}
	n, err := parseSize(v.Raw)
	return n, v.Version, err
}

// parseSize decodes a size value; an empty value is 0, and a value beyond the
// int64 range is an error.
func parseSize(raw []byte) (uint64, error) {
	if len(raw) == 0 {
		return 0, nil
	}
	n, err := strconv.ParseUint(string(raw), 10, 64)
	if err != nil {
		return 0, errors.Wrapf(err, "invalid log size [%s]", raw)
	}
	// Sizes and indexes become int64 in tlog and in TransparencyLogEntry.
	if n > math.MaxInt64 {
		return 0, errors.Errorf("log size %d exceeds the int64 range", n)
	}
	return n, nil
}

func formatSize(n uint64) []byte {
	return []byte(strconv.FormatUint(n, 10))
}

// logServiceOf returns this node's LogService.
func logServiceOf(viewCtx view.Context) (*LogService, error) {
	s, err := viewCtx.GetService(reflect.TypeFor[*LogService]())
	if err != nil {
		return nil, errors.Wrap(err, "failed getting the log service")
	}
	ls, ok := s.(*LogService)
	if !ok {
		return nil, errors.Errorf("unexpected log service type [%T]", s)
	}
	return ls, nil
}

// queryServiceOf returns the committer query service of the default channel.
func queryServiceOf(viewCtx view.Context) (queryservice.QueryService, error) {
	network, ch, err := fabric.GetDefaultChannel(viewCtx)
	if err != nil {
		return nil, errors.Wrap(err, "failed getting the default channel")
	}
	qs, err := queryservice.GetQueryService(viewCtx, network.Name(), ch.Name())
	if err != nil {
		return nil, errors.Wrap(err, "failed getting the query service")
	}
	return qs, nil
}
