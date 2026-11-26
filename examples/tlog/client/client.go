/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// Package client is the transparency-log demo's "fake cosign": a client
// that signs an artifact, logs it through one tlog node, and verifies
// everything that node hands back against all three orgs' keys, so it
// never has to trust the node it talks to.
package client

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/json"

	rekorv1 "github.com/sigstore/protobuf-specs/gen/pb-go/rekor/v1"
	pb "github.com/sigstore/rekor-tiles/v2/pkg/generated/protobuf"
	"github.com/sigstore/rekor-tiles/v2/pkg/verify"
	"golang.org/x/mod/sumdb/note"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/hyperledger-labs/fabric-smart-client/examples/tlog/translog"
	"github.com/hyperledger-labs/fabric-smart-client/examples/tlog/views"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/api"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
)

// Client is a transparency-log client: it signs artifacts with Key, logs
// and reads them through Node, and checks everything Node returns against
// Verifiers, the checkpoint verifiers of all three orgs.
type Client struct {
	// Node is the tlog node this client talks to.
	Node api.ViewClient
	// Key signs the artifacts this client logs.
	Key *ecdsa.PrivateKey
	// Verifiers are the three orgs' checkpoint verifiers, in Org1..Org3
	// order.
	Verifiers []note.Verifier
	// Latest is the checkpoint the last successful Audit or Log left
	// verified, nil until then.
	Latest []byte
}

// Log signs sha256(artifact) with c.Key, logs it through c.Node, verifies
// the entry it gets back offline against c.Verifiers, audits the log, and
// returns the entry.
func (c *Client) Log(ctx context.Context, artifact []byte) (*rekorv1.TransparencyLogEntry, error) {
	digest := sha256.Sum256(artifact)
	sig, err := ecdsa.SignASN1(rand.Reader, c.Key, digest[:])
	if err != nil {
		return nil, errors.Wrap(err, "failed signing the artifact")
	}
	pubDER, err := x509.MarshalPKIXPublicKey(&c.Key.PublicKey)
	if err != nil {
		return nil, errors.Wrap(err, "failed marshaling the public key")
	}
	req, err := protojson.Marshal(translog.NewRequest(digest[:], sig, pubDER))
	if err != nil {
		return nil, errors.Wrap(err, "failed marshaling the log request")
	}

	raw, err := c.call(ctx, "log", &views.LogRequest{Request: req})
	if err != nil {
		return nil, err
	}
	entry := &rekorv1.TransparencyLogEntry{}
	if err := protojson.Unmarshal(raw, entry); err != nil {
		return nil, errors.Wrap(err, "failed decoding the logged entry")
	}
	if err := Verify(entry, artifact, &c.Key.PublicKey, c.Verifiers); err != nil {
		return nil, err
	}
	if err := c.Audit(ctx); err != nil {
		return nil, err
	}
	return entry, nil
}

// Get fetches the entry at index through c.Node and verifies it offline
// against a checkpoint signed by all three orgs before returning it.
func (c *Client) Get(ctx context.Context, index uint64) (*rekorv1.TransparencyLogEntry, error) {
	raw, err := c.call(ctx, "get", &views.GetRequest{Index: index})
	if err != nil {
		return nil, err
	}
	entry := &rekorv1.TransparencyLogEntry{}
	if err := protojson.Unmarshal(raw, entry); err != nil {
		return nil, errors.Wrap(err, "failed decoding the entry")
	}
	// VerifyInclusionProof proves the leaf at entry.LogIndex, whatever that
	// is; without this check a node could answer any index with some other,
	// honestly provable entry.
	li := entry.GetLogIndex()
	if li < 0 || uint64(li) != index {
		return nil, errors.Errorf("entry is for index %d, not the requested index %d", li, index)
	}
	if entry.GetInclusionProof().GetLogIndex() != li {
		return nil, errors.Errorf("inclusion proof is for index %d, not the entry's index %d", entry.GetInclusionProof().GetLogIndex(), li)
	}
	cp, err := translog.VerifyAll([]byte(entry.GetInclusionProof().GetCheckpoint().GetEnvelope()), c.Verifiers)
	if err != nil {
		return nil, err
	}
	if err := verify.VerifyInclusionProof(entry, cp); err != nil {
		return nil, errors.Wrap(err, "failed verifying the inclusion proof")
	}
	return entry, nil
}

// Audit asks c.Node for the current checkpoint, checks it carries every
// org's signature, proves it consistent with c.Latest if any, and on
// success replaces c.Latest with it.
func (c *Client) Audit(ctx context.Context) error {
	var oldSize uint64
	if c.Latest != nil {
		oldCp, err := translog.VerifyAll(c.Latest, c.Verifiers)
		if err != nil {
			return errors.Wrap(err, "failed verifying the previously audited checkpoint")
		}
		oldSize = oldCp.Size
	}

	raw, err := c.call(ctx, "checkpoint", &views.CheckpointRequest{OldSize: oldSize})
	if err != nil {
		return err
	}
	resp := &views.CheckpointResponse{}
	if err := json.Unmarshal(raw, resp); err != nil {
		return errors.Wrap(err, "failed decoding the checkpoint response")
	}
	if _, err := translog.VerifyAll(resp.Checkpoint, c.Verifiers); err != nil {
		return errors.Wrap(err, "failed verifying the checkpoint")
	}
	if c.Latest != nil {
		// VerifyConsistencyProofWithCheckpoints only checks one verifier's
		// signature on each checkpoint; VerifyAll above already checked all
		// three on the new one, and did so on the old one when it was
		// stored as Latest.
		if err := verify.VerifyConsistencyProofWithCheckpoints(resp.Consistency, string(c.Latest), string(resp.Checkpoint), c.Verifiers[0]); err != nil {
			return errors.Wrap(err, "failed verifying consistency with the previous checkpoint")
		}
	}
	c.Latest = resp.Checkpoint
	return nil
}

// Verify checks e offline, without trusting the node it came from:
// CanonicalizedBody must equal the canonical entry of sha256(artifact), the
// signature CanonicalizedBody itself carries, and pub; the checkpoint in
// its inclusion proof must carry every verifier in vs's signature; and the
// inclusion proof against that checkpoint must hold.
func Verify(e *rekorv1.TransparencyLogEntry, artifact []byte, pub *ecdsa.PublicKey, vs []note.Verifier) error {
	sig, err := bodySignature(e.GetCanonicalizedBody())
	if err != nil {
		return err
	}
	pubDER, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return errors.Wrap(err, "failed marshaling the public key")
	}
	digest := sha256.Sum256(artifact)
	body, err := translog.CanonicalEntry(translog.NewRequest(digest[:], sig, pubDER))
	if err != nil {
		return errors.Wrap(err, "failed rebuilding the entry")
	}
	if !bytes.Equal(body, e.GetCanonicalizedBody()) {
		return errors.New("entry body does not match the signed artifact")
	}

	cp, err := translog.VerifyAll([]byte(e.GetInclusionProof().GetCheckpoint().GetEnvelope()), vs)
	if err != nil {
		return err
	}
	if err := verify.VerifyInclusionProof(e, cp); err != nil {
		return errors.Wrap(err, "failed verifying the inclusion proof")
	}
	return nil
}

// bodySignature extracts the signature content a hashedrekord canonical
// entry body carries.
func bodySignature(body []byte) ([]byte, error) {
	entry := &pb.Entry{}
	if err := protojson.Unmarshal(body, entry); err != nil {
		return nil, errors.Wrap(err, "failed decoding the entry body")
	}
	sig := entry.GetSpec().GetHashedRekordV002().GetSignature().GetContent()
	if len(sig) == 0 {
		return nil, errors.New("entry body has no signature")
	}
	return sig, nil
}

// call runs the view registered as fid on c.Node and returns its raw
// result.
func (c *Client) call(ctx context.Context, fid string, in any) ([]byte, error) {
	payload, err := json.Marshal(in)
	if err != nil {
		return nil, errors.Wrapf(err, "failed marshaling the %s request", fid)
	}
	res, err := c.Node.CallViewWithContext(ctx, fid, payload)
	if err != nil {
		return nil, errors.Wrapf(err, "failed calling the %s view", fid)
	}
	raw, ok := res.([]byte)
	if !ok {
		return nil, errors.Errorf("unexpected %s view result type [%T]", fid, res)
	}
	return raw, nil
}
