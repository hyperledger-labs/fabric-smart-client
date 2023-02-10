/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"bytes"
	"context"
	"encoding/base64"
	"strconv"
	"strings"
	"sync"
	"time"

	peer2 "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/peer"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/transaction"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/hyperledger/fabric-protos-go/common"
	pb "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
	"go.uber.org/zap/zapcore"
)

type Invoke struct {
	Chaincode                      *Chaincode
	ServiceProvider                view2.ServiceProvider
	Network                        Network
	Channel                        Channel
	TxID                           driver.TxID
	SignerIdentity                 view.Identity
	ChaincodePath                  string
	ChaincodeName                  string
	ChaincodeVersion               string
	TransientMap                   map[string][]byte
	EndorsersMSPIDs                []string
	ImplicitCollectionMSPIDs       []string
	EndorsersFromMyOrg             bool
	EndorsersByConnConfig          []*grpc.ConnectionConfig
	DiscoveredEndorsersByEndpoints []string
	Function                       string
	Args                           []interface{}
	MatchEndorsementPolicy         bool
	NumRetries                     int
	RetrySleep                     time.Duration
	Context                        context.Context
}

func NewInvoke(chaincode *Chaincode, function string, args ...interface{}) *Invoke {
	return &Invoke{
		Chaincode:       chaincode,
		ServiceProvider: chaincode.sp,
		Network:         chaincode.network,
		Channel:         chaincode.channel,
		ChaincodeName:   chaincode.name,
		Function:        function,
		Args:            args,
		NumRetries:      int(chaincode.NumRetries),
		RetrySleep:      chaincode.RetrySleep,
	}
}

func (i *Invoke) Endorse() (driver.Envelope, error) {
	for j := 0; j < i.NumRetries; j++ {
		res, err := i.endorse()
		if err != nil {
			if j+1 >= i.NumRetries {
				return nil, err
			}
			time.Sleep(i.RetrySleep)
			continue
		}
		return res, nil
	}
	return nil, errors.Errorf("failed to perform endorse")
}

func (i *Invoke) endorse() (driver.Envelope, error) {
	_, prop, responses, signer, err := i.prepare(false)
	if err != nil {
		return nil, err
	}

	proposalResp := responses[0]
	if proposalResp == nil {
		return nil, errors.New("error during query: received nil proposal response")
	}

	// assemble a signed transaction (it's an Envelope message)
	env, err := protoutil.CreateSignedTx(prop, signer, responses...)
	if err != nil {
		return nil, errors.WithMessage(err, "could not assemble transaction")
	}

	return transaction.NewEnvelopeFromEnv(env)
}

func (i *Invoke) Query() ([]byte, error) {
	for j := 0; j < i.NumRetries; j++ {
		res, err := i.query()
		if err != nil {
			if j+1 >= i.NumRetries {
				return nil, err
			}
			time.Sleep(i.RetrySleep)
			continue
		}
		return res, nil
	}
	return nil, errors.Errorf("failed to perform query")
}

func (i *Invoke) query() ([]byte, error) {
	_, _, responses, _, err := i.prepare(!i.MatchEndorsementPolicy)
	if err != nil {
		return nil, err
	}

	// check all responses match
	resp := responses[0]
	if resp == nil {
		return nil, errors.New("error during query: received nil proposal response")
	}
	if resp.Endorsement == nil {
		return nil, errors.Errorf("endorsement failure during query. endorsement is nil: [%v]", resp.Response)
	}
	if resp.Response == nil {
		return nil, errors.Errorf("endorsement failure during query. response is nil: [%v]", resp.Endorsement)
	}
	for i := 1; i < len(responses); i++ {
		if responses[i].Endorsement == nil {
			return nil, errors.Errorf("endorsement failure during query. endorsement is nil: [%v]", resp.Response)
		}
		if responses[i].Response == nil {
			return nil, errors.Errorf("endorsement failure during query. response is nil: [%v]", resp.Endorsement)
		}
		if !bytes.Equal(responses[i].Response.Payload, resp.Response.Payload) {
			return nil, errors.Errorf("endorsement failure during query. response payload does not match")
		}
		if responses[i].Response.Status != resp.Response.Status {
			return nil, errors.Errorf("endorsement failure during query. response status does not match")
		}
		if responses[i].Response.Message != resp.Response.Message {
			return nil, errors.Errorf("endorsement failure during query. response message does not match")
		}
	}

	return resp.Response.Payload, nil
}

func (i *Invoke) Submit() (string, []byte, error) {
	for j := 0; j < i.NumRetries; j++ {
		txID, res, err := i.submit()
		if err != nil {
			if j+1 >= i.NumRetries {
				return "", nil, err
			}
			time.Sleep(i.RetrySleep)
			continue
		}
		return txID, res, nil
	}
	return "", nil, errors.Errorf("failed to perform submit")
}

func (i *Invoke) submit() (string, []byte, error) {
	txID, prop, responses, signer, err := i.prepare(false)
	if err != nil {
		return "", nil, err
	}

	proposalResp := responses[0]
	if proposalResp == nil {
		return "", nil, errors.New("error during query: received nil proposal response")
	}

	// assemble a signed transaction (it's an Envelope message)
	env, err := protoutil.CreateSignedTx(prop, signer, responses...)
	if err != nil {
		return txID, proposalResp.Response.Payload, errors.WithMessage(err, "could not assemble transaction")
	}

	// Broadcast envelope and wait for finality
	err = i.broadcast(txID, env)
	if err != nil {
		return "", nil, err
	}

	return txID, proposalResp.Response.Payload, nil
}

func (i *Invoke) WithContext(context context.Context) driver.ChaincodeInvocation {
	i.Context = context
	return i
}

func (i *Invoke) WithTransientEntry(k string, v interface{}) driver.ChaincodeInvocation {
	if i.TransientMap == nil {
		i.TransientMap = map[string][]byte{}
	}
	b, err := i.toBytes(v)
	if err != nil {
		panic(err)
	}
	i.TransientMap[k] = b
	return i
}

func (i *Invoke) WithEndorsersByMSPIDs(mspIDs ...string) driver.ChaincodeInvocation {
	i.EndorsersMSPIDs = mspIDs
	return i
}

func (i *Invoke) WithEndorsersFromMyOrg() driver.ChaincodeInvocation {
	i.EndorsersFromMyOrg = true
	return i
}

// WithDiscoveredEndorsersByEndpoints sets the endpoints to be used to filter the result of
// discovery. Discovery is used to identify the chaincode's endorsers, if not set otherwise.
func (i *Invoke) WithDiscoveredEndorsersByEndpoints(endpoints ...string) driver.ChaincodeInvocation {
	i.DiscoveredEndorsersByEndpoints = endpoints
	return i
}

func (i *Invoke) WithMatchEndorsementPolicy() driver.ChaincodeInvocation {
	i.MatchEndorsementPolicy = true
	return i
}

func (i *Invoke) WithSignerIdentity(id view.Identity) driver.ChaincodeInvocation {
	i.SignerIdentity = id
	return i
}

func (i *Invoke) WithEndorsersByConnConfig(ccs ...*grpc.ConnectionConfig) driver.ChaincodeInvocation {
	i.EndorsersByConnConfig = ccs
	return i
}

func (i *Invoke) WithImplicitCollections(mspIDs ...string) driver.ChaincodeInvocation {
	i.ImplicitCollectionMSPIDs = mspIDs
	return i
}

func (i *Invoke) WithTxID(id driver.TxID) driver.ChaincodeInvocation {
	i.TxID = id
	return i
}

func (i *Invoke) WithNumRetries(numRetries uint) driver.ChaincodeInvocation {
	i.NumRetries = int(numRetries)
	return i
}

func (i *Invoke) WithRetrySleep(duration time.Duration) driver.ChaincodeInvocation {
	i.RetrySleep = duration
	return i
}

func (i *Invoke) prepare(query bool) (string, *pb.Proposal, []*pb.ProposalResponse, driver.SigningIdentity, error) {
	// TODO: improve by providing grpc connection pool
	var peerClients []peer2.Client
	defer func() {
		for _, pCli := range peerClients {
			pCli.Close()
		}
	}()

	if i.SignerIdentity.IsNone() {
		return "", nil, nil, nil, errors.Errorf("no invoker specified")
	}
	if len(i.ChaincodeName) == 0 {
		return "", nil, nil, nil, errors.Errorf("no chaincode specified")
	}

	// load endorser clients
	var endorserClients []pb.EndorserClient
	var discoveredPeers []driver.DiscoveredPeer
	switch {
	case len(i.EndorsersByConnConfig) != 0:
		// get a peer client for each connection config
		for _, config := range i.EndorsersByConnConfig {
			peerClient, err := i.Channel.NewPeerClientForAddress(*config)
			if err != nil {
				return "", nil, nil, nil, err
			}
			peerClients = append(peerClients, peerClient)
		}
	default:
		if i.EndorsersFromMyOrg && len(i.EndorsersMSPIDs) == 0 {
			// retrieve invoker's MSP-ID
			invokerMSPID, err := i.Channel.MSPManager().DeserializeIdentity(i.SignerIdentity)
			if err != nil {
				return "", nil, nil, nil, errors.WithMessagef(err, "failed to deserializer the invoker identity")
			}
			i.EndorsersMSPIDs = []string{invokerMSPID.GetMSPIdentifier()}
		}

		// discover
		var err error
		discovery := NewDiscovery(
			i.Chaincode,
		)
		discovery.WithFilterByMSPIDs(
			i.EndorsersMSPIDs...,
		).WithImplicitCollections(
			i.ImplicitCollectionMSPIDs...,
		)
		if query {
			discovery.WithForQuery()
		}
		peers, err := discovery.Call()
		if err != nil {
			return "", nil, nil, nil, err
		}
		if len(i.DiscoveredEndorsersByEndpoints) != 0 {
			for _, peer := range peers {
				for _, endpoint := range i.DiscoveredEndorsersByEndpoints {
					if peer.Endpoint == endpoint {
						// append
						discoveredPeers = append(discoveredPeers, peer)
						break
					}
				}
			}
		} else {
			discoveredPeers = peers
		}
	}

	// get a peer client for all discovered peers
	for _, peer := range discoveredPeers {
		peerClient, err := i.Channel.NewPeerClientForAddress(grpc.ConnectionConfig{
			Address:          peer.Endpoint,
			TLSEnabled:       i.Network.Config().TLSEnabled(),
			TLSRootCertBytes: peer.TLSRootCerts,
		})
		if err != nil {
			return "", nil, nil, nil, errors.WithMessagef(err, "error getting endorser client for %s", peer.Endpoint)
		}
		peerClients = append(peerClients, peerClient)
	}

	// get endorser clients
	for _, client := range peerClients {
		endorserClient, err := client.Endorser()
		if err != nil {
			return "", nil, nil, nil, errors.WithMessagef(err, "error getting endorser client for %s", client.Address())
		}
		endorserClients = append(endorserClients, endorserClient)
	}
	if len(endorserClients) == 0 {
		return "", nil, nil, nil, errors.New("no endorser clients retrieved with the current filters")
	}

	// load signer
	signer, err := i.Network.SignerService().GetSigningIdentity(i.SignerIdentity)
	if err != nil {
		return "", nil, nil, nil, err
	}

	// prepare proposal
	signedProp, prop, txID, err := i.prepareProposal(signer)
	if err != nil {
		return "", nil, nil, nil, err
	}

	// collect responses
	responses, err := i.collectResponses(endorserClients, signedProp)
	if err != nil {
		return "", nil, nil, nil, errors.Wrapf(err, "failed collecting proposal responses")
	}

	if len(responses) == 0 {
		// this should only happen if some new code has introduced a bug
		return "", nil, nil, nil, errors.New("no proposal responses received - this might indicate a bug")
	}

	return txID, prop, responses, signer, nil
}

func (i *Invoke) prepareProposal(signer SerializableSigner) (*pb.SignedProposal, *pb.Proposal, string, error) {
	spec, err := i.getChaincodeSpec()
	if err != nil {
		return nil, nil, "", err
	}
	invocation := &pb.ChaincodeInvocationSpec{ChaincodeSpec: spec}

	creator, err := signer.Serialize()
	if err != nil {
		return nil, nil, "", errors.WithMessage(err, "error serializing identity")
	}
	funcName := "invoke"
	prop, txID, err := i.createChaincodeProposalWithTxIDAndTransient(
		common.HeaderType_ENDORSER_TRANSACTION,
		i.Channel.Name(),
		invocation,
		creator,
		i.TransientMap)
	if err != nil {
		return nil, nil, "", errors.WithMessagef(err, "error creating proposal for %s", funcName)
	}
	signedProp, err := protoutil.GetSignedProposal(prop, signer)
	if err != nil {
		return nil, nil, "", errors.WithMessagef(err, "error creating signed proposal for %s", funcName)
	}

	return signedProp, prop, txID, nil
}

// createChaincodeProposalWithTxIDAndTransient creates a proposal from given
// input. It returns the proposal and the transaction id associated with the
// proposal
func (i *Invoke) createChaincodeProposalWithTxIDAndTransient(typ common.HeaderType, channelID string, cis *pb.ChaincodeInvocationSpec, creator []byte, transientMap map[string][]byte) (*pb.Proposal, string, error) {
	var nonce []byte
	var txID string

	if len(i.TxID.Creator) == 0 {
		i.TxID.Creator = creator
	}
	if len(i.TxID.Nonce) == 0 {
		if logger.IsEnabledFor(zapcore.DebugLevel) {
			logger.Debugf("generate nonce and tx-id for [%s,%s]", view.Identity(i.TxID.Creator).String(), base64.StdEncoding.EncodeToString(nonce))
		}
		txID = transaction.ComputeTxID(&i.TxID)
		nonce = i.TxID.Nonce
	} else {
		nonce = i.TxID.Nonce
		txID = transaction.ComputeTxID(&i.TxID)
		if logger.IsEnabledFor(zapcore.DebugLevel) {
			logger.Debugf("no need to generate nonce and tx-id [%s,%s]", base64.StdEncoding.EncodeToString(nonce), txID)
		}
	}

	if logger.IsEnabledFor(zapcore.DebugLevel) {
		logger.Debugf("create chaincode proposal with tx id [%s], nonce [%s]", txID, base64.StdEncoding.EncodeToString(nonce))
	}

	return protoutil.CreateChaincodeProposalWithTxIDNonceAndTransient(txID, typ, channelID, cis, nonce, creator, transientMap)
}

// collectResponses sends a signed proposal to a set of peers, and gathers all the responses.
func (i *Invoke) collectResponses(endorserClients []pb.EndorserClient, signedProposal *pb.SignedProposal) ([]*pb.ProposalResponse, error) {
	responsesCh := make(chan *pb.ProposalResponse, len(endorserClients))
	errorCh := make(chan error, len(endorserClients))
	wg := sync.WaitGroup{}
	for _, endorser := range endorserClients {
		wg.Add(1)
		go func(endorser pb.EndorserClient) {
			defer wg.Done()
			proposalResp, err := endorser.ProcessProposal(context.Background(), signedProposal)
			if err != nil {
				errorCh <- err
				return
			}
			responsesCh <- proposalResp
		}(endorser)
	}
	wg.Wait()
	close(responsesCh)
	close(errorCh)
	for err := range errorCh {
		return nil, err
	}
	var responses []*pb.ProposalResponse
	for response := range responsesCh {
		responses = append(responses, response)
	}
	return responses, nil
}

// getChaincodeSpec get chaincode spec from the fsccli cmd parameters
func (i *Invoke) getChaincodeSpec() (*pb.ChaincodeSpec, error) {
	// prepare args
	args, err := i.prepareArgs()
	if err != nil {
		return nil, err
	}

	// Build the spec
	input := &pb.ChaincodeInput{
		Args:        args,
		Decorations: nil,
		IsInit:      false,
	}
	chaincodeLang := "golang"
	chaincodeLang = strings.ToUpper(chaincodeLang)
	spec := &pb.ChaincodeSpec{
		Type:        pb.ChaincodeSpec_Type(pb.ChaincodeSpec_Type_value[chaincodeLang]),
		ChaincodeId: &pb.ChaincodeID{Path: i.ChaincodePath, Name: i.ChaincodeName, Version: i.ChaincodeVersion},
		Input:       input,
	}
	return spec, nil
}

func (i *Invoke) prepareArgs() ([][]byte, error) {
	var args [][]byte
	args = append(args, []byte(i.Function))
	for _, arg := range i.Args {
		b, err := i.toBytes(arg)
		if err != nil {
			return nil, err
		}
		args = append(args, b)
	}
	return args, nil
}

func (i *Invoke) toBytes(arg interface{}) ([]byte, error) {
	switch v := arg.(type) {
	case []byte:
		return v, nil
	case string:
		return []byte(v), nil
	case int:
		return []byte(strconv.Itoa(v)), nil
	case int64:
		return []byte(strconv.FormatInt(v, 10)), nil
	case uint64:
		return []byte(strconv.FormatUint(v, 10)), nil
	default:
		return nil, errors.Errorf("arg type [%T] not recognized.", v)
	}
}

func (i *Invoke) broadcast(txID string, env *common.Envelope) error {
	if err := i.Network.Broadcast(i.Context, env); err != nil {
		return err
	}
	return i.Channel.IsFinal(context.Background(), txID)
}
