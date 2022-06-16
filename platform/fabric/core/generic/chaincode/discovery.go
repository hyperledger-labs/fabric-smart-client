/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"context"
	"strings"
	"time"

	peer2 "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/peer"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	discovery2 "github.com/hyperledger/fabric-protos-go/discovery"
	"github.com/hyperledger/fabric/common/util"
	discovery "github.com/hyperledger/fabric/discovery/client"
	"github.com/pkg/errors"
)

const (
	defaultTimeout = time.Second * 10
)

type Discovery struct {
	chaincode *Chaincode

	FilterByMSPIDs      []string
	ImplicitCollections []string

	DefaultTTL time.Duration
}

func NewDiscovery(chaincode *Chaincode) *Discovery {
	// set key to the concatenation of chaincode name and version
	return &Discovery{
		chaincode:  chaincode,
		DefaultTTL: 5 * time.Minute,
	}
}

func (d *Discovery) Call() ([]driver.DiscoveredPeer, error) {
	var sb strings.Builder
	sb.WriteString(d.chaincode.network.Name())
	sb.WriteString(d.chaincode.channel.Name())
	sb.WriteString(d.chaincode.name)
	for _, mspiD := range d.FilterByMSPIDs {
		sb.WriteString(mspiD)
	}
	key := sb.String()

	var response discovery.Response

	// Do we have a response already?
	d.chaincode.discoveryResultsCacheLock.RLock()
	responseBoxed, err := d.chaincode.discoveryResultsCache.Get(key)
	if responseBoxed != nil && err == nil {
		response = responseBoxed.(discovery.Response)
	}
	d.chaincode.discoveryResultsCacheLock.RUnlock()

	if response == nil {
		// fetch the response
		response, err = d.send()
		if err != nil {
			return nil, errors.WithMessage(err, "failed to send discovery request")
		}
	}

	// extract endorsers
	cr := response.ForChannel(d.chaincode.channel.Name())
	var endorsers discovery.Endorsers
	switch {
	case len(d.ImplicitCollections) > 0:
		for _, collection := range d.ImplicitCollections {
			discoveredEndorsers, err := cr.Endorsers(
				ccCall(d.chaincode.name),
				&byMSPIDs{mspIDs: []string{collection}},
			)
			if err != nil {
				return nil, errors.WithMessage(err, "failed to get endorsers")
			}
			endorsers = append(endorsers, discoveredEndorsers...)
		}
	default:
		endorsers, err = cr.Endorsers(
			ccCall(d.chaincode.name),
			&byMSPIDs{mspIDs: d.FilterByMSPIDs},
		)
	}
	if err != nil {
		return nil, errors.WithMessagef(err, "failed getting endorsers for [%s]", key)
	}
	configResult, err := cr.Config()
	if err != nil {
		return nil, errors.WithMessagef(err, "failed getting config for [%s]", key)
	}

	var discoveredEndorsers []driver.DiscoveredPeer
	for _, peer := range endorsers {
		// extract peer info
		member := peer.AliveMessage.GetAliveMsg().Membership
		if member == nil {
			return nil, errors.Errorf("member is nil for [%s:%s]", peer.MSPID, view.Identity(peer.Identity).String())
		}
		var tlsRootCerts [][]byte
		if mspInfo, ok := configResult.GetMsps()[peer.MSPID]; ok {
			tlsRootCerts = append(tlsRootCerts, mspInfo.GetTlsRootCerts()...)
			tlsRootCerts = append(tlsRootCerts, mspInfo.GetTlsIntermediateCerts()...)
		}
		discoveredEndorsers = append(discoveredEndorsers, driver.DiscoveredPeer{
			Identity:     peer.Identity,
			MSPID:        peer.MSPID,
			Endpoint:     member.Endpoint,
			TLSRootCerts: tlsRootCerts,
		})
	}

	// cache response
	d.chaincode.discoveryResultsCacheLock.Lock()
	defer d.chaincode.discoveryResultsCacheLock.Unlock()
	if err := d.chaincode.discoveryResultsCache.SetWithTTL(key, response, d.DefaultTTL); err != nil {
		logger.Warnf("failed to set discovery results in cache: %s", err)
	}

	// done
	return discoveredEndorsers, nil
}

func (d *Discovery) WithFilterByMSPIDs(mspIDs ...string) driver.ChaincodeDiscover {
	d.FilterByMSPIDs = mspIDs
	return d
}

func (d *Discovery) WithImplicitCollections(mspIDs ...string) driver.ChaincodeDiscover {
	d.ImplicitCollections = mspIDs
	return d
}

func (d *Discovery) send() (discovery.Response, error) {
	var peerClients []peer2.Client
	defer func() {
		for _, pCli := range peerClients {
			pCli.Close()
		}
	}()

	// New discovery request for:
	// - endorsers,
	// - config, and
	// - local peers (for alive message)
	req, err := discovery.NewRequest().OfChannel(d.chaincode.channel.Name()).AddEndorsersQuery(
		&discovery2.ChaincodeInterest{Chaincodes: []*discovery2.ChaincodeCall{{Name: d.chaincode.name}}},
	)
	if err != nil {
		return nil, errors.Wrap(err, "failed creating request")
	}
	req = req.AddConfigQuery().AddLocalPeersQuery()

	pc, err := d.chaincode.channel.NewPeerClientForAddress(*d.chaincode.network.PickPeer())
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
	timeout, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()
	cl, err := pc.DiscoveryClient()
	if err != nil {
		return nil, errors.Wrap(err, "failed creating discovery client")
	}
	response, err := cl.Send(timeout, req, &discovery2.AuthInfo{
		ClientIdentity:    signerRaw,
		ClientTlsCertHash: ClientTLSCertHash,
	})
	if err != nil {
		return nil, errors.WithMessage(err, "failed requesting endorsers")
	}

	return response, nil
}

func ccCall(ccNames ...string) []*discovery2.ChaincodeCall {
	var call []*discovery2.ChaincodeCall
	for _, ccName := range ccNames {
		call = append(call, &discovery2.ChaincodeCall{
			Name: ccName,
		})
	}
	return call
}

type byMSPIDs struct {
	mspIDs []string
}

func (f *byMSPIDs) Filter(endorsers discovery.Endorsers) discovery.Endorsers {
	if len(f.mspIDs) == 0 {
		return endorsers
	}

	var filteredEndorsers discovery.Endorsers
	for _, endorser := range endorsers {
		endorserMSPID := endorser.MSPID
		found := false
		for _, mspID := range f.mspIDs {
			if mspID == endorserMSPID {
				found = true
				break
			}
		}
		if !found {
			continue
		}
		filteredEndorsers = append(filteredEndorsers, endorser)
	}
	return filteredEndorsers
}
