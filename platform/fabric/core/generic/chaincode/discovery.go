/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"context"
	"reflect"
	"strings"
	"time"

	discovery2 "github.com/hyperledger/fabric-protos-go/discovery"
	"github.com/hyperledger/fabric/common/util"
	discovery "github.com/hyperledger/fabric/discovery/client"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
	"go.uber.org/zap/zapcore"

	peer2 "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/peer"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

const (
	defaultTimeout = time.Second * 10
)

// ServiceResponse represents a response sent from the discovery service
type ServiceResponse interface {
	// ForChannel returns a ChannelResponse in the context of a given channel
	ForChannel(string) discovery.ChannelResponse

	// ForLocal returns a LocalResponse in the context of no channel
	ForLocal() discovery.LocalResponse

	// Raw returns the raw response from the server
	Raw() *discovery.Response
}

type Discovery struct {
	chaincode      *Chaincode
	filterByMSPIDs []string

	defaultTTL time.Duration
}

func NewDiscovery(chaincode *Chaincode) *Discovery {
	// set key to the concatenation of chaincode name and version
	return &Discovery{
		chaincode:  chaincode,
		defaultTTL: 5 * time.Minute,
	}
}

func (d *Discovery) Call() ([]view.Identity, error) {
	var sb strings.Builder
	sb.WriteString(d.chaincode.network.Name())
	sb.WriteString(d.chaincode.channel.Name())
	sb.WriteString(d.chaincode.name)
	for _, mspiD := range d.filterByMSPIDs {
		sb.WriteString(mspiD)
	}
	key := sb.String()

	// TODO: Do we have an answer already?
	d.chaincode.discoveryResultsCacheLock.RLock()
	endorsersBoxed, err := d.chaincode.discoveryResultsCache.Get(key)
	if endorsersBoxed != nil && err == nil {
		endorsers := endorsersBoxed.([]view.Identity)
		if len(endorsers) != 0 {
			d.chaincode.discoveryResultsCacheLock.RUnlock()
			return endorsers, nil
		}
	}
	d.chaincode.discoveryResultsCacheLock.RUnlock()

	// TODO: improve by providing grpc connection pool
	var peerClients []peer2.Client
	defer func() {
		for _, pCli := range peerClients {
			pCli.Close()
		}
	}()

	req, err := discovery.NewRequest().OfChannel(d.chaincode.channel.Name()).AddEndorsersQuery(
		&discovery2.ChaincodeInterest{Chaincodes: []*discovery2.ChaincodeCall{
			{
				Name: d.chaincode.name,
			},
		}},
	)
	if err != nil {
		return nil, errors.Wrap(err, "failed creating request")
	}

	pc, err := d.chaincode.channel.NewPeerClientForAddress(*d.chaincode.network.Peers()[0])
	if err != nil {
		return nil, err
	}
	peerClients = append(peerClients, pc)

	signer := d.chaincode.network.LocalMembership().DefaultSigningIdentity()
	signerRaw, err := signer.Serialize()
	if err != nil {
		return nil, err
	}

	var ClientTLSCertHash []byte
	if len(pc.Certificate().Certificate) != 0 {
		ClientTLSCertHash = util.ComputeSHA256(pc.Certificate().Certificate[0])
	}
	req.Authentication = &discovery2.AuthInfo{
		ClientIdentity:    signerRaw,
		ClientTlsCertHash: ClientTLSCertHash,
	}
	payload := protoutil.MarshalOrPanic(req.Request)
	sig, err := signer.Sign(payload)
	if err != nil {
		return nil, err
	}

	timeout, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()
	dc, err := pc.Discovery()
	if err != nil {
		return nil, err
	}
	res, err := dc.Discover(timeout, &discovery2.SignedRequest{
		Payload:   payload,
		Signature: sig,
	})
	if err != nil {
		return nil, err
	}

	if len(res.Results) == 0 {
		return nil, errors.New("empty results")
	}

	if e := res.Results[0].GetError(); e != nil {
		return nil, errors.Errorf("server returned: %s", e.Content)
	}

	ccQueryRes := res.Results[0].GetCcQueryRes()
	if ccQueryRes == nil {
		return nil, errors.Errorf("server returned response of unexpected type: %v", reflect.TypeOf(res.Results[0]))
	}

	mspManager := d.chaincode.channel.MSPManager()

	endorserSet := make(map[string][]byte)
	for _, descriptor := range ccQueryRes.Content {
		for _, layout := range descriptor.Layouts {
			for group, q := range layout.QuantitiesByGroup {
				// Peek q peers from descriptor.EndorsersByGroups[group].Peers

				for i := 0; i < int(q); i++ {
					endorserID := descriptor.EndorsersByGroups[group].Peers[i].Identity
					if logger.IsEnabledFor(zapcore.DebugLevel) {
						logger.Debugf("endorser discovered [%s,%s] [%s]", descriptor.Chaincode, group, view.Identity(endorserID))
					}

					if len(d.filterByMSPIDs) != 0 {
						endorser, err := mspManager.DeserializeIdentity(endorserID)
						if err != nil {
							return nil, errors.WithMessagef(err, "failed deserializing identity [%s]", view.Identity(endorserID).String())
						}
						endorserMSPID := endorser.GetMSPIdentifier()
						found := false
						for _, mspID := range d.filterByMSPIDs {
							if mspID == endorserMSPID {
								found = true
								break
							}
						}
						if !found {
							continue
						}
					}

					endorserSet[string(endorserID)] = endorserID
				}
			}
		}
	}

	var endorsers []view.Identity
	for _, e := range endorserSet {
		endorsers = append(endorsers, e)
	}

	d.chaincode.discoveryResultsCacheLock.Lock()
	defer d.chaincode.discoveryResultsCacheLock.Unlock()
	if err := d.chaincode.discoveryResultsCache.SetWithTTL(key, endorsers, d.defaultTTL); err != nil {
		logger.Warnf("failed to set discovery results in cache: %s", err)
	}

	return endorsers, nil
}

func (d *Discovery) WithFilterByMSPIDs(mspIDs ...string) driver.ChaincodeDiscover {
	d.filterByMSPIDs = mspIDs
	return d
}
