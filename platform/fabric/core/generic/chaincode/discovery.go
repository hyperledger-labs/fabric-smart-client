/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"context"
	"crypto/sha256"
	"strings"
	"sync"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/cache"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/discovery"
	peer2 "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/services"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	discovery2 "github.com/hyperledger/fabric-protos-go-apiv2/discovery"
	"github.com/hyperledger/fabric-protos-go-apiv2/peer"
)

const (
	DiscoveryCacheTimeout = time.Minute
)

type Discovery struct {
	chaincode *Chaincode

	FilterByMSPIDs      []string
	ImplicitCollections []string
	QueryForPeers       bool

	discoveryResultsCacheLock sync.RWMutex
	discoveryResultsCache     cache.Map[string, discovery.Response]
}

// NewDiscovery create a discovery client that helps to fetch peer information.
//
// Discovery results are cached. The TTL of the cached results can be set via the `ChannelConfig.DiscoveryTimeout()`
// of the chaincode parameter. If not set, the default `DiscoveryCacheTimeout` is used.
func NewDiscovery(chaincode *Chaincode) *Discovery {
	timeout := DiscoveryCacheTimeout
	if chaincode != nil || chaincode.ChannelConfig != nil || chaincode.ChannelConfig.DiscoveryTimeout() > 0 {
		timeout = chaincode.ChannelConfig.DiscoveryDefaultTTLS()
	}

	return &Discovery{
		chaincode:             chaincode,
		discoveryResultsCache: cache.NewTimeoutCache[string, discovery.Response](timeout, nil),
	}
}

func (d *Discovery) Call() ([]driver.DiscoveredPeer, error) {
	if d.QueryForPeers {
		return d.GetPeers()
	}
	return d.GetEndorsers()
}

func (d *Discovery) GetEndorsers() ([]driver.DiscoveredPeer, error) {
	response, err := d.Response()
	if err != nil {
		return nil, errors.WithMessagef(err, "failed to get discovery response")
	}

	// extract endorsers
	cr := response.ForChannel(d.chaincode.ChannelID)
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
		return nil, errors.WithMessagef(err, "failed getting endorsers for [%s:%s:%s]", d.chaincode.NetworkID, d.chaincode.ChannelID, d.chaincode.name)
	}

	// prepare result
	configResult, err := cr.Config()
	if err != nil {
		return nil, errors.WithMessagef(err, "failed getting config for [%s:%s:%s]", d.chaincode.NetworkID, d.chaincode.ChannelID, d.chaincode.name)
	}
	return d.toDiscoveredPeers(configResult, endorsers)
}

func (d *Discovery) GetPeers() ([]driver.DiscoveredPeer, error) {
	response, err := d.Response()
	if err != nil {
		return nil, errors.WithMessagef(err, "failed to get discovery response")
	}

	// extract peers
	cr := response.ForChannel(d.chaincode.ChannelID)
	var peers []*discovery.Peer
	peers, err = cr.Peers(ccCall(d.chaincode.name)...)
	if err != nil {
		return nil, errors.WithMessagef(err, "failed getting peers for [%s:%s:%s]", d.chaincode.NetworkID, d.chaincode.ChannelID, d.chaincode.name)
	}

	// filter
	switch {
	case len(d.ImplicitCollections) > 0:
		for _, collection := range d.ImplicitCollections {
			peers = (&byMSPIDs{mspIDs: []string{collection}}).Filter(peers)
		}
	default:
		peers = (&byMSPIDs{mspIDs: d.FilterByMSPIDs}).Filter(peers)
	}

	// prepare result
	configResult, err := cr.Config()
	if err != nil {
		return nil, errors.WithMessagef(err, "failed getting config for [%s:%s:%s]", d.chaincode.NetworkID, d.chaincode.ChannelID, d.chaincode.name)
	}
	return d.toDiscoveredPeers(configResult, peers)
}

func (d *Discovery) Response() (discovery.Response, error) {
	var sb strings.Builder
	sb.WriteString(d.chaincode.NetworkID)
	sb.WriteString(d.chaincode.ChannelID)
	sb.WriteString(d.chaincode.name)
	for _, mspiD := range d.FilterByMSPIDs {
		sb.WriteString(mspiD)
	}
	if d.QueryForPeers {
		sb.WriteString("QueryForPeers")
	}
	key := sb.String()

	// Do we have a response already?
	d.discoveryResultsCacheLock.RLock()
	resp, ok := d.discoveryResultsCache.Get(key)
	d.discoveryResultsCacheLock.RUnlock()
	if ok {
		return resp, nil
	}

	d.discoveryResultsCacheLock.Lock()
	defer d.discoveryResultsCacheLock.Unlock()

	if resp, ok := d.discoveryResultsCache.Get(key); ok {
		return resp, nil
	}

	// fetch the response
	var response discovery.Response
	var err error
	if d.QueryForPeers {
		response, err = d.queryPeers()
	} else {
		response, err = d.queryEndorsers()
	}
	if err != nil {
		return nil, errors.WithMessage(err, "failed to send discovery request")
	}

	// cache response
	d.discoveryResultsCache.Put(key, response)

	// done
	return response, nil
}

func (d *Discovery) WithForQuery() driver.ChaincodeDiscover {
	d.QueryForPeers = true
	return d
}

func (d *Discovery) WithFilterByMSPIDs(mspIDs ...string) driver.ChaincodeDiscover {
	d.FilterByMSPIDs = mspIDs
	return d
}

func (d *Discovery) WithImplicitCollections(mspIDs ...string) driver.ChaincodeDiscover {
	d.ImplicitCollections = mspIDs
	return d
}

func (d *Discovery) queryPeers() (discovery.Response, error) {
	// New discovery request for:
	// - peers and
	// - config,
	req := discovery.NewRequest().OfChannel(d.chaincode.ChannelID).AddPeersQuery(
		&peer.ChaincodeCall{Name: d.chaincode.name},
	)
	req = req.AddConfigQuery()
	return d.query(req)
}

func (d *Discovery) queryEndorsers() (discovery.Response, error) {
	// New discovery request for:
	// - endorsers and
	// - config,
	req, err := discovery.NewRequest().OfChannel(d.chaincode.ChannelID).AddEndorsersQuery(
		&peer.ChaincodeInterest{Chaincodes: []*peer.ChaincodeCall{{Name: d.chaincode.name}}},
	)
	if err != nil {
		return nil, errors.Wrap(err, "failed creating request")
	}
	req = req.AddConfigQuery()
	return d.query(req)
}

func (d *Discovery) query(req *discovery.Request) (discovery.Response, error) {
	var peerClients []peer2.Client
	defer func() {
		for _, pCli := range peerClients {
			pCli.Close()
		}
	}()
	pc, err := d.chaincode.Services.NewPeerClient(*d.chaincode.ConfigService.PickPeer(driver.PeerForDiscovery))
	if err != nil {
		return nil, err
	}
	peerClients = append(peerClients, pc)

	signer := d.chaincode.LocalMembership.DefaultSigningIdentity()
	signerRaw, err := signer.Serialize()
	if err != nil {
		return nil, err
	}
	var ClientTLSCertHash []byte
	if len(pc.Certificate().Certificate) != 0 {
		h := sha256.Sum256(pc.Certificate().Certificate[0])
		ClientTLSCertHash = h[:]
	}
	req.Authentication = &discovery2.AuthInfo{
		ClientIdentity:    signerRaw,
		ClientTlsCertHash: ClientTLSCertHash,
	}
	timeout, cancel := context.WithTimeout(context.Background(), d.chaincode.ChannelConfig.DiscoveryTimeout())
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

func (d *Discovery) toDiscoveredPeers(configResult *discovery2.ConfigResult, endorsers []*discovery.Peer) ([]driver.DiscoveredPeer, error) {
	var discoveredEndorsers []driver.DiscoveredPeer
	for _, peer := range endorsers {
		// extract peer info
		if peer.AliveMessage == nil {
			continue
		}
		aliveMsg := peer.AliveMessage.GetAliveMsg()
		if aliveMsg == nil {
			continue
		}
		member := aliveMsg.Membership
		if member == nil {
			logger.Debugf("no membership info in alive message for peer [%s:%s]", peer.MSPID, view.Identity(peer.Identity).String())
			continue
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

	return discoveredEndorsers, nil
}

func (d *Discovery) ChaincodeVersion() (string, error) {
	response, err := d.Response()
	if err != nil {
		return "", errors.Wrapf(err, "unable to discover channel information for chaincode [%s] on channel [%s]", d.chaincode.name, d.chaincode.ChannelID)
	}
	endorsers, err := response.ForChannel(d.chaincode.ChannelID).Endorsers([]*peer.ChaincodeCall{{
		Name: d.chaincode.name,
	}}, &noFilter{})
	if err != nil {
		return "", errors.Wrapf(err, "failed to get endorsers for chaincode [%s] on channel [%s]", d.chaincode.name, d.chaincode.ChannelID)
	}
	if len(endorsers) == 0 {
		return "", errors.Errorf("no endorsers found for chaincode [%s] on channel [%s]", d.chaincode.name, d.chaincode.ChannelID)
	}
	stateInfoMessage := endorsers[0].StateInfoMessage
	if stateInfoMessage == nil {
		return "", errors.Errorf("no state info message found for chaincode [%s] on channel [%s]", d.chaincode.name, d.chaincode.ChannelID)
	}
	stateInfo := stateInfoMessage.GetStateInfo()
	if stateInfo == nil {
		return "", errors.Errorf("no state info found for chaincode [%s] on channel [%s]", d.chaincode.name, d.chaincode.ChannelID)
	}
	properties := stateInfo.GetProperties()
	if properties == nil {
		return "", errors.Errorf("no properties found for chaincode [%s] on channel [%s]", d.chaincode.name, d.chaincode.ChannelID)
	}
	chaincodes := properties.Chaincodes
	if len(chaincodes) == 0 {
		return "", errors.Errorf("no chaincode info found for chaincode [%s] on channel [%s]", d.chaincode.name, d.chaincode.ChannelID)
	}
	for _, chaincode := range chaincodes {
		if chaincode.Name == d.chaincode.name {
			return chaincode.Version, nil
		}
	}
	return "", errors.Errorf("chaincode [%s] not found", d.chaincode.name)
}

func ccCall(ccNames ...string) []*peer.ChaincodeCall {
	var call []*peer.ChaincodeCall
	for _, ccName := range ccNames {
		call = append(call, &peer.ChaincodeCall{
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

type noFilter struct{}

func (f *noFilter) Filter(endorsers discovery.Endorsers) discovery.Endorsers {
	return endorsers
}
