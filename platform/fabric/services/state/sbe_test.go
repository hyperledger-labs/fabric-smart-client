/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package state

import (
	"errors"
	"testing"

	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/hyperledger/fabric-protos-go-apiv2/msp"
	"github.com/hyperledger/fabric-protos-go-apiv2/peer"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// mspRolePolicy builds a serialized SignaturePolicyEnvelope containing one ROLE
// principal per mspid, which is the shape newStateEP knows how to read back.
func mspRolePolicy(tb testing.TB, mspids ...string) []byte {
	tb.Helper()

	principals := make([]*msp.MSPPrincipal, 0, len(mspids))
	for _, id := range mspids {
		raw, err := proto.Marshal(&msp.MSPRole{Role: msp.MSPRole_MEMBER, MspIdentifier: id})
		require.NoError(tb, err)
		principals = append(principals, &msp.MSPPrincipal{
			PrincipalClassification: msp.MSPPrincipal_ROLE,
			Principal:               raw,
		})
	}

	raw, err := proto.Marshal(&common.SignaturePolicyEnvelope{Identities: principals})
	require.NoError(tb, err)
	return raw
}

func TestNewStateEPEmptyPolicy(t *testing.T) {
	t.Parallel()

	ep, err := newStateEP(nil)
	require.NoError(t, err)
	require.Empty(t, ep.listOrgs())
	require.Empty(t, ep.identities)
}

func TestNewStateEPFromPolicy(t *testing.T) {
	t.Parallel()

	ep, err := newStateEP(mspRolePolicy(t, "Org2MSP", "Org1MSP"))
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"Org1MSP", "Org2MSP"}, ep.listOrgs())
}

func TestNewStateEPMalformedPolicy(t *testing.T) {
	t.Parallel()

	_, err := newStateEP([]byte("not-a-proto"))
	require.Error(t, err)
	require.ErrorContains(t, err, "error unmarshaling to SignaturePolicy")
}

// TestNewStateEPMalformedPrincipal covers the inner unmarshal in setMSPIDsFromSP:
// the envelope parses, but a ROLE principal's payload does not.
func TestNewStateEPMalformedPrincipal(t *testing.T) {
	t.Parallel()

	raw, err := proto.Marshal(&common.SignaturePolicyEnvelope{
		Identities: []*msp.MSPPrincipal{{
			PrincipalClassification: msp.MSPPrincipal_ROLE,
			Principal:               []byte("not-an-msp-role"),
		}},
	})
	require.NoError(t, err)

	_, err = newStateEP(raw)
	require.Error(t, err)
	require.ErrorContains(t, err, "error unmarshaling msp principal")
}

// TestNewStateEPIgnoresNonRolePrincipals records that only ROLE principals are
// read back; an IDENTITY principal is skipped rather than rejected.
func TestNewStateEPIgnoresNonRolePrincipals(t *testing.T) {
	t.Parallel()

	raw, err := proto.Marshal(&common.SignaturePolicyEnvelope{
		Identities: []*msp.MSPPrincipal{{
			PrincipalClassification: msp.MSPPrincipal_IDENTITY,
			Principal:               []byte("some-identity"),
		}},
	})
	require.NoError(t, err)

	ep, err := newStateEP(raw)
	require.NoError(t, err)
	require.Empty(t, ep.listOrgs(), "IDENTITY principals do not contribute orgs")
}

func TestStateEPPolicyRoundTrip(t *testing.T) {
	t.Parallel()

	ep, err := newStateEP(mspRolePolicy(t, "Org1MSP", "Org2MSP"))
	require.NoError(t, err)

	policy, err := ep.Policy()
	require.NoError(t, err)
	require.NotEmpty(t, policy)

	back, err := newStateEP(policy)
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"Org1MSP", "Org2MSP"}, back.listOrgs())
}

// TestStateEPPolicyOrgsAreSorted pins the deterministic ordering, which matters
// because the policy bytes are compared across peers.
func TestStateEPPolicyOrgsAreSorted(t *testing.T) {
	t.Parallel()

	first, err := newStateEP(mspRolePolicy(t, "OrgB", "OrgA", "OrgC"))
	require.NoError(t, err)
	firstBytes, err := first.Policy()
	require.NoError(t, err)

	second, err := newStateEP(mspRolePolicy(t, "OrgC", "OrgA", "OrgB"))
	require.NoError(t, err)
	secondBytes, err := second.Policy()
	require.NoError(t, err)

	require.Equal(t, firstBytes, secondBytes,
		"the same org set must serialize identically regardless of input order")
}

func TestStateEPPolicyWithIdentities(t *testing.T) {
	t.Parallel()

	ep, err := newStateEP(nil)
	require.NoError(t, err)
	ep.addOwner(view.Identity("alice"))
	ep.addOwner(view.Identity("bob"))

	policy, err := ep.Policy()
	require.NoError(t, err)

	spe := &common.SignaturePolicyEnvelope{}
	require.NoError(t, proto.Unmarshal(policy, spe))
	require.Len(t, spe.Identities, 2)
	require.Equal(t, int32(2), spe.Rule.GetNOutOf().N,
		"every principal must sign")
}

// TestStateEPPolicyMixesOrgsAndIdentities checks both principal kinds end up in
// one envelope, orgs first.
func TestStateEPPolicyMixesOrgsAndIdentities(t *testing.T) {
	t.Parallel()

	ep, err := newStateEP(mspRolePolicy(t, "Org1MSP"))
	require.NoError(t, err)
	ep.addOwner(view.Identity("alice"))

	policy, err := ep.Policy()
	require.NoError(t, err)

	spe := &common.SignaturePolicyEnvelope{}
	require.NoError(t, proto.Unmarshal(policy, spe))
	require.Len(t, spe.Identities, 2)
	require.Equal(t, msp.MSPPrincipal_ROLE, spe.Identities[0].PrincipalClassification)
	require.Equal(t, msp.MSPPrincipal_IDENTITY, spe.Identities[1].PrincipalClassification)
}

func TestStateEPPolicyEmpty(t *testing.T) {
	t.Parallel()

	ep, err := newStateEP(nil)
	require.NoError(t, err)

	policy, err := ep.Policy()
	require.NoError(t, err)

	spe := &common.SignaturePolicyEnvelope{}
	require.NoError(t, proto.Unmarshal(policy, spe))
	require.Empty(t, spe.Identities)
	require.Equal(t, int32(0), spe.Rule.GetNOutOf().N)
}

// --- sbeMetaHandler -------------------------------------------------------

func TestSBEMetaHandlerSkippedWhenNotRequested(t *testing.T) {
	t.Parallel()

	tx, rwset, _ := newTestStateTransaction("sbens")
	handler := &sbeMetaHandler{forceSBE: false}

	require.NoError(t, handler.StoreMeta(tx.Namespace, &House{Owner: view.Identity("o")}, "sbens", "k1",
		&addOutputOptions{sbe: false}))

	meta, err := rwset.GetStateMetadata("sbens", "k1")
	require.NoError(t, err)
	require.Empty(t, meta, "no policy is written when SBE is not requested")
}

func TestSBEMetaHandlerSkippedForNonOwnableState(t *testing.T) {
	t.Parallel()

	tx, rwset, _ := newTestStateTransaction("sbens")
	handler := &sbeMetaHandler{forceSBE: true}

	// Asset does not implement Ownable.
	require.NoError(t, handler.StoreMeta(tx.Namespace, &Asset{ID: "a1"}, "sbens", "k1", &addOutputOptions{}))

	meta, err := rwset.GetStateMetadata("sbens", "k1")
	require.NoError(t, err)
	require.Empty(t, meta, "a state without owners gets no endorsement policy")
}

func TestSBEMetaHandlerWritesValidationParameter(t *testing.T) {
	t.Parallel()

	tx, rwset, _ := newTestStateTransaction("sbens")
	handler := &sbeMetaHandler{forceSBE: true}

	require.NoError(t, handler.StoreMeta(tx.Namespace,
		&House{Owner: view.Identity("owner-1")}, "sbens", "k1", &addOutputOptions{}))

	meta, err := rwset.GetStateMetadata("sbens", "k1")
	require.NoError(t, err)
	require.Contains(t, meta, peer.MetaDataKeys_VALIDATION_PARAMETER.String())
	require.NotEmpty(t, meta[peer.MetaDataKeys_VALIDATION_PARAMETER.String()])
}

// TestSBEMetaHandlerPreservesExistingMetadata checks the policy is merged into
// existing metadata rather than replacing the whole map.
func TestSBEMetaHandlerPreservesExistingMetadata(t *testing.T) {
	t.Parallel()

	tx, rwset, _ := newTestStateTransaction("sbens")
	require.NoError(t, rwset.SetStateMetadata("sbens", "k1", map[string][]byte{"keep": []byte("me")}))

	handler := &sbeMetaHandler{forceSBE: true}
	require.NoError(t, handler.StoreMeta(tx.Namespace,
		&House{Owner: view.Identity("owner-1")}, "sbens", "k1", &addOutputOptions{}))

	meta, err := rwset.GetStateMetadata("sbens", "k1")
	require.NoError(t, err)
	require.Equal(t, []byte("me"), meta["keep"], "unrelated metadata survives")
	require.Contains(t, meta, peer.MetaDataKeys_VALIDATION_PARAMETER.String())
}

// TestSBEMetaHandlerOptionEnablesSBE covers the per-output opt-in, as distinct
// from the handler-wide forceSBE flag.
func TestSBEMetaHandlerOptionEnablesSBE(t *testing.T) {
	t.Parallel()

	tx, rwset, _ := newTestStateTransaction("sbens")
	handler := &sbeMetaHandler{forceSBE: false}

	require.NoError(t, handler.StoreMeta(tx.Namespace,
		&House{Owner: view.Identity("owner-1")}, "sbens", "k1", &addOutputOptions{sbe: true}))

	meta, err := rwset.GetStateMetadata("sbens", "k1")
	require.NoError(t, err)
	require.Contains(t, meta, peer.MetaDataKeys_VALIDATION_PARAMETER.String())
}

func TestSBEMetaHandlerRWSetError(t *testing.T) {
	t.Parallel()

	tx, _, driverTx := newTestStateTransaction("sbens")
	driverTx.getRWSetErr = errors.New("rwset failed")
	handler := &sbeMetaHandler{forceSBE: true}

	err := handler.StoreMeta(tx.Namespace, &House{Owner: view.Identity("o")}, "sbens", "k1",
		&addOutputOptions{})
	require.Error(t, err)
	require.ErrorContains(t, err, "getting rw set")
}

func TestSBEMetaHandlerGetMetadataError(t *testing.T) {
	t.Parallel()

	tx, rwset, _ := newTestStateTransaction("sbens")
	rwset.getStateMetadataErr = errors.New("read metadata failed")
	handler := &sbeMetaHandler{forceSBE: true}

	err := handler.StoreMeta(tx.Namespace, &House{Owner: view.Identity("o")}, "sbens", "k1",
		&addOutputOptions{})
	require.Error(t, err)
	require.ErrorContains(t, err, "getting metadata")
}

func TestSBEMetaHandlerSetMetadataError(t *testing.T) {
	t.Parallel()

	tx, rwset, _ := newTestStateTransaction("sbens")
	rwset.setStateMetadataErr = errors.New("write metadata failed")
	handler := &sbeMetaHandler{forceSBE: true}

	err := handler.StoreMeta(tx.Namespace, &House{Owner: view.Identity("o")}, "sbens", "k1",
		&addOutputOptions{})
	require.Error(t, err)
	require.ErrorContains(t, err, "failed setting metadata")
}
