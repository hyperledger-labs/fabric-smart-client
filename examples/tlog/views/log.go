/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views

import (
	"bytes"
	"context"
	"encoding/json"
	"slices"
	"time"

	rekorv1 "github.com/sigstore/protobuf-specs/gen/pb-go/rekor/v1"
	pb "github.com/sigstore/rekor-tiles/v2/pkg/generated/protobuf"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/hyperledger-labs/fabric-smart-client/examples/tlog/translog"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	cdriver "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/state"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/committer/queryservice"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

var logger = logging.MustGetLogger()

const (
	logCommand = "log"
	// requestTransient is the transient key carrying the request to endorsers.
	requestTransient = "request"
	// logRetries is how often a log transaction invalidated by a concurrent
	// one is rebuilt.
	logRetries      = 3
	finalityTimeout = 30 * time.Second
)

// errInvalid marks a log transaction the committer found invalid, usually
// because a concurrent one changed the size first.
var errInvalid = errors.New("transaction invalid")

// errStale marks a log transaction an endorser rejected, usually because it
// was built on a size that a concurrent one changed before endorsement.
var errStale = errors.New("endorsement refused")

// LogRequest is the input of the log view.
type LogRequest struct {
	// Request is a protojson HashedRekordRequestV002.
	Request []byte
}

// LogView logs an entry and returns it as a protojson TransparencyLogEntry,
// with an inclusion proof against a checkpoint all three orgs signed.
type LogView struct {
	LogRequest
}

func (l *LogView) Call(viewCtx view.Context) (any, error) {
	req, err := decodeRequest(l.Request)
	if err != nil {
		return nil, err
	}
	body, err := translog.CanonicalEntry(req)
	if err != nil {
		return nil, err
	}
	ls, err := logServiceOf(viewCtx)
	if err != nil {
		return nil, err
	}
	_, _, peers, err := ls.keys()
	if err != nil {
		return nil, err
	}

	var index uint64
	for attempt := 0; ; attempt++ {
		index, err = l.submit(viewCtx, body, peers)
		if err == nil {
			break
		}
		retryable := errors.Is(err, errInvalid) || errors.Is(err, errStale)
		if !retryable || attempt == logRetries {
			return nil, err
		}
		logger.Infof("log transaction not committed, retrying: %v", err)
		// The context keeps a session per initiator view and party; close
		// them so the retry opens new endorsement sessions.
		for _, peer := range peers {
			if s, err := viewCtx.GetSession(viewCtx.Initiator(), peer); err == nil {
				s.Close()
			}
		}
	}

	return buildEntry(viewCtx, ls, index)
}

// buildEntry returns the entry at index as a protojson TransparencyLogEntry,
// with an inclusion proof against a checkpoint all three orgs signed.
// Requires index < the log's current size.
func buildEntry(viewCtx view.Context, ls *LogService, index uint64) ([]byte, error) {
	raw, err := ls.Checkpoint(viewCtx)
	if err != nil {
		return nil, err
	}
	_, verifiers, _, err := ls.keys()
	if err != nil {
		return nil, err
	}
	cp, err := translog.VerifyAll(raw, verifiers)
	if err != nil {
		return nil, err
	}
	qs, err := queryServiceOf(viewCtx)
	if err != nil {
		return nil, err
	}
	body, err := readEntry(qs, index)
	if err != nil {
		return nil, err
	}
	hashes, err := ls.Tree().InclusionProof(index, cp.Size)
	if err != nil {
		return nil, err
	}
	return protojson.Marshal(&rekorv1.TransparencyLogEntry{
		LogIndex:          int64(index), //nolint:gosec // a log never reaches 2^63 entries
		CanonicalizedBody: body,
		KindVersion:       &rekorv1.KindVersion{Kind: "hashedrekord", Version: "0.0.2"},
		InclusionProof: &rekorv1.InclusionProof{
			LogIndex:   int64(index),   //nolint:gosec // see above
			TreeSize:   int64(cp.Size), //nolint:gosec // see above
			RootHash:   cp.Hash,
			Hashes:     hashes,
			Checkpoint: &rekorv1.Checkpoint{Envelope: string(raw)},
		},
	})
}

// readEntry reads the canonicalized body of the entry at index.
func readEntry(qs queryservice.QueryService, index uint64) ([]byte, error) {
	v, err := qs.GetState(Namespace, EntryKey(index))
	if err != nil {
		return nil, errors.Wrap(err, "failed reading the entry")
	}
	if v == nil || len(v.Raw) == 0 {
		return nil, errors.Errorf("entry [%s] is missing", EntryKey(index))
	}
	return v.Raw, nil
}

// submit appends body at the end of the log in one transaction endorsed by
// this node and peers, and returns its index once committed.
func (l *LogView) submit(viewCtx view.Context, body []byte, peers []view.Identity) (uint64, error) {
	tx, err := state.NewTransaction(viewCtx)
	if err != nil {
		return 0, errors.Wrap(err, "failed creating a transaction")
	}
	tx.SetNamespace(Namespace)
	if err := tx.AddCommand(logCommand); err != nil {
		return 0, errors.Wrap(err, "failed adding the log command")
	}
	if err := tx.SetTransient(requestTransient, l.Request); err != nil {
		return 0, errors.Wrap(err, "failed attaching the request")
	}
	rws, err := tx.Transaction.RWSet()
	if err != nil {
		return 0, errors.Wrap(err, "failed getting the rwset")
	}
	// Reading size makes a concurrent log transaction invalidate this one.
	raw, err := rws.GetState(Namespace, SizeKey)
	if err != nil {
		return 0, errors.Wrap(err, "failed reading the log size")
	}
	n, err := parseSize(raw)
	if err != nil {
		return 0, err
	}
	if err := rws.SetState(Namespace, EntryKey(n), body); err != nil {
		return 0, errors.Wrap(err, "failed writing the entry")
	}
	if err := rws.SetState(Namespace, SizeKey, formatSize(n+1)); err != nil {
		return 0, errors.Wrap(err, "failed writing the log size")
	}

	me := tx.FabricNetworkService().IdentityProvider().DefaultIdentity()
	if _, err := viewCtx.RunView(state.NewCollectEndorsementsView(tx, append([]view.Identity{me}, peers...)...)); err != nil {
		return 0, errors.Wrapf(errStale, "failed collecting endorsements: %v", err)
	}

	// Through the committer: only its listener manager answers for a
	// transaction it already reported on.
	_, ch, err := fabric.GetDefaultChannel(viewCtx)
	if err != nil {
		return 0, errors.Wrap(err, "failed getting the default channel")
	}
	lm := ch.Committer()
	listener := statusListener(make(chan fdriver.ValidationCode, 1))
	if err := lm.AddFinalityListener(tx.ID(), listener); err != nil {
		return 0, errors.Wrap(err, "failed adding a finality listener")
	}
	defer func() {
		if err := lm.RemoveFinalityListener(tx.ID(), listener); err != nil {
			logger.Warnf("failed removing the finality listener of [%s]: %v", tx.ID(), err)
		}
	}()

	// An invalid transaction fails here too; the listener tells why.
	_, orderErr := viewCtx.RunView(state.NewOrderingAndFinalityWithTimeoutView(tx, finalityTimeout))
	select {
	case vc := <-listener:
		switch vc {
		case fdriver.Valid:
			return n, nil
		case fdriver.Invalid:
			return 0, errors.Wrapf(errInvalid, "tx [%s]", tx.ID())
		default:
			return 0, errors.Errorf("tx [%s] is %s", tx.ID(), fdriver.StatusName(vc))
		}
	case <-time.After(finalityTimeout):
		if orderErr != nil {
			return 0, errors.WithMessagef(orderErr, "failed ordering tx [%s]", tx.ID())
		}
		return 0, errors.Errorf("tx [%s]: no status reported", tx.ID())
	}
}

// statusListener receives the first status of one transaction.
type statusListener chan fdriver.ValidationCode

func (l statusListener) OnStatus(_ context.Context, _ cdriver.TxID, vc fdriver.ValidationCode, _ string) {
	select {
	case l <- vc:
	default:
	}
}

// LogViewFactory creates a LogView from a JSON LogRequest.
type LogViewFactory struct{}

func (*LogViewFactory) NewView(in []byte) (view.View, error) {
	f := &LogView{}
	if err := json.Unmarshal(in, &f.LogRequest); err != nil {
		return nil, errors.Wrap(err, "failed decoding the input")
	}
	return f, nil
}

// LogResponderView endorses a log transaction: it rebuilds the entry from
// the request itself and checks that the transaction appends exactly that
// entry at the end of the log.
type LogResponderView struct{}

func (*LogResponderView) Call(viewCtx view.Context) (any, error) {
	tx, err := state.ReceiveTransaction(viewCtx)
	if err != nil {
		return nil, errors.Wrap(err, "failed receiving the transaction")
	}
	// Only a party allowed to propose on the channel may log.
	_, ch, err := fabric.GetChannel(viewCtx, tx.Network(), tx.Channel())
	if err != nil {
		return nil, errors.Wrap(err, "failed getting the channel")
	}
	if err := ch.ACLProvider().CheckACL(tx.SignedProposal()); err != nil {
		return nil, errors.Wrap(err, "proposer failed the channel ACL")
	}
	if err := checkLogTransaction(viewCtx, tx); err != nil {
		return nil, errors.WithMessagef(err, "rejected log transaction [%s]", tx.ID())
	}
	if _, err := viewCtx.RunView(state.NewEndorseView(tx)); err != nil {
		return nil, errors.WithMessage(err, "failed endorsing")
	}
	return nil, nil
}

// checkLogTransaction reads what tx writes and validates it against the
// log size this node reads itself.
func checkLogTransaction(viewCtx view.Context, tx *state.Transaction) error {
	cmds, err := tx.Commands()
	if err != nil {
		return errors.Wrap(err, "failed reading the commands")
	}
	commands := make([]string, cmds.Count())
	for i := range commands {
		commands[i] = cmds.At(i).Name
	}

	rws, err := tx.Transaction.RWSet()
	if err != nil {
		return errors.Wrap(err, "failed getting the rwset")
	}
	for _, ns := range rws.Namespaces() {
		if ns != Namespace && rws.NumWrites(ns) > 0 {
			return errors.Errorf("writes to namespace [%s]", ns)
		}
	}
	reads := make([]string, rws.NumReads(Namespace))
	for i := range reads {
		if reads[i], err = rws.GetReadKeyAt(Namespace, i); err != nil {
			return errors.Wrapf(err, "failed reading read %d", i)
		}
	}
	writes := map[string][]byte{}
	for i := range rws.NumWrites(Namespace) {
		key, value, err := rws.GetWriteAt(Namespace, i)
		if err != nil {
			return errors.Wrapf(err, "failed reading write %d", i)
		}
		writes[key] = value
	}

	var readVersion cdriver.RawVersion
	if slices.Contains(reads, SizeKey) {
		if readVersion, err = rws.GetReadVersion(Namespace, SizeKey); err != nil {
			return errors.Wrapf(err, "failed reading the read version of [%s]", SizeKey)
		}
	}

	qs, err := queryServiceOf(viewCtx)
	if err != nil {
		return err
	}
	n, ledgerVersion, err := readSize(qs)
	if err != nil {
		return err
	}
	return validateLogTx(commands, reads, readVersion, ledgerVersion, writes, tx.GetTransient(requestTransient), n)
}

// validateLogTx accepts a log transaction only if its single command is
// log, it reads size in Namespace at ledgerVersion, and it writes exactly two
// keys there: the entry rebuilt from request at index ledgerSize, and
// size = ledgerSize+1. ledgerSize and ledgerVersion are what this node reads
// itself; readVersion is the version the transaction reads size at, nil for
// a missing key, as is ledgerVersion.
//
// The read of size is what makes the committer reject a second transaction
// built on the same size; without it both writes would be blind and the
// later one would overwrite the entry. Its version must be the one this node
// reads: otherwise a proposer reading size at a newer version could get a
// lagging endorser to approve writing an entry that already exists, and the
// committer would accept it, since the read is current.
func validateLogTx(commands, reads []string, readVersion, ledgerVersion cdriver.RawVersion, writes map[string][]byte, request []byte, ledgerSize uint64) error {
	if len(commands) != 1 || commands[0] != logCommand {
		return errors.Errorf("expected the single command [%s], got %v", logCommand, commands)
	}
	if !slices.Contains(reads, SizeKey) {
		return errors.Errorf("no read of [%s]", SizeKey)
	}
	if !bytes.Equal(readVersion, ledgerVersion) {
		return errors.Errorf("[%s] read at version [%x], want [%x]", SizeKey, readVersion, ledgerVersion)
	}
	req, err := decodeRequest(request)
	if err != nil {
		return err
	}
	body, err := translog.CanonicalEntry(req)
	if err != nil {
		return err
	}
	if len(writes) != 2 {
		return errors.Errorf("expected 2 writes, got %d", len(writes))
	}
	entry, ok := writes[EntryKey(ledgerSize)]
	if !ok {
		return errors.Errorf("no write to [%s], the end of the log", EntryKey(ledgerSize))
	}
	if !bytes.Equal(entry, body) {
		return errors.Errorf("[%s] is not the entry of the request", EntryKey(ledgerSize))
	}
	if size := writes[SizeKey]; !bytes.Equal(size, formatSize(ledgerSize+1)) {
		return errors.Errorf("size written as [%s], want %d", size, ledgerSize+1)
	}
	return nil
}

// decodeRequest decodes a protojson HashedRekordRequestV002.
func decodeRequest(raw []byte) (*pb.HashedRekordRequestV002, error) {
	if len(raw) == 0 {
		return nil, errors.New("failed decoding the request: it is missing")
	}
	req := &pb.HashedRekordRequestV002{}
	if err := protojson.Unmarshal(raw, req); err != nil {
		return nil, errors.Wrap(err, "failed decoding the request")
	}
	return req, nil
}
