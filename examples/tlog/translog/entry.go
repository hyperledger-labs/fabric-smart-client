/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// Package translog holds the pure, FSC-free log logic of the transparency-log
// demo: turning a signed hash into the canonical entry bytes that go into the
// Merkle tree, the same way Rekor does.
package translog

import (
	"github.com/cyberphone/json-canonicalization/go/src/webpki.org/jsoncanonicalizer"
	v1 "github.com/sigstore/protobuf-specs/gen/pb-go/common/v1"
	pb "github.com/sigstore/rekor-tiles/v2/pkg/generated/protobuf"
	"github.com/sigstore/rekor-tiles/v2/pkg/types/hashedrekord"
	"github.com/sigstore/sigstore/pkg/signature"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
)

// keyDetails is the only signing algorithm this demo supports: ECDSA on
// P-256 over a SHA-256 digest, Rekor's default for hashedrekord entries.
const keyDetails = v1.PublicKeyDetails_PKIX_ECDSA_P256_SHA_256

// NewRequest builds a HashedRekordRequestV002 for digest, signed by sig, whose
// signer is identified by its DER-encoded (PKIX) public key publicKeyDER. The
// request always names PKIX_ECDSA_P256_SHA_256 as its key and signing
// algorithm, the only one this demo signs with.
func NewRequest(digest, sig, publicKeyDER []byte) *pb.HashedRekordRequestV002 {
	return &pb.HashedRekordRequestV002{
		Digest: digest,
		Signature: &pb.Signature{
			Content: sig,
			Verifier: &pb.Verifier{
				Verifier:   &pb.Verifier_PublicKey{PublicKey: &pb.PublicKey{RawBytes: publicKeyDER}},
				KeyDetails: keyDetails,
			},
		},
	}
}

// CanonicalEntry verifies req's signature and returns the canonicalized JSON
// bytes of its Rekor log entry: the Merkle leaf that
// rfc6962.DefaultHasher.HashLeaf hashes into the tree.
func CanonicalEntry(req *pb.HashedRekordRequestV002) ([]byte, error) {
	registry, err := signature.NewAlgorithmRegistryConfig([]v1.PublicKeyDetails{keyDetails})
	if err != nil {
		return nil, errors.Wrap(err, "building algorithm registry")
	}
	entry, err := hashedrekord.ToLogEntry(req, registry)
	if err != nil {
		return nil, errors.Wrap(err, "converting request to log entry")
	}
	serialized, err := protojson.Marshal(entry)
	if err != nil {
		return nil, errors.Wrap(err, "marshaling log entry")
	}
	canonicalized, err := jsoncanonicalizer.Transform(serialized)
	if err != nil {
		return nil, errors.Wrap(err, "canonicalizing log entry")
	}
	return canonicalized, nil
}
