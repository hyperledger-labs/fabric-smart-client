/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package queryservice

import (
	"context"
	"sync/atomic"
	"time"

	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/hyperledger/fabric-x-common/api/applicationpb"
	"github.com/hyperledger/fabric-x-common/api/committerpb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/committer/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/connstate"
)

var (
	logger = logging.MustGetLogger()

	ErrInvalidQueryInput = errors.New("invalid query input")
)

type RemoteQueryService struct {
	// newClient returns the stub for one request. It is called per request, so each
	// request can land on a different endpoint (see config.ClientProvider).
	newClient func() (committerpb.QueryServiceClient, error)
	config    *config.Config
	newConn   func() *connstate.Tracker
	conn      atomic.Pointer[connstate.Tracker]
}

// NewRemoteQueryService returns a query service that calls clientFactory for the stub of
// every request, and returns its error unchanged when it fails.
// The outcome of each query is reported to the tracker newConn returns: connected when the
// query service answers, including with an error, and reconnecting when it is unavailable.
// A query that times out does not change the state. newConn is called on the first answered
// query, so the series exists only once the service has been reached, and a query service
// that is unused or never reached reports nothing. A nil newConn reports nothing.
func NewRemoteQueryService(config *config.Config, clientFactory func() (committerpb.QueryServiceClient, error), newConn func() *connstate.Tracker) *RemoteQueryService {
	return &RemoteQueryService{
		newClient: clientFactory,
		config:    config,
		newConn:   newConn,
	}
}

// report records the connection state implied by the error of a query.
func (s *RemoteQueryService) report(err error) {
	switch status.Code(err) {
	case codes.Unavailable:
		s.conn.Load().Set(connstate.Reconnecting) // no-op until the first answer
	case codes.DeadlineExceeded:
		// A slow answer says nothing about the connection.
	default:
		t := s.conn.Load()
		if t == nil && s.newConn != nil {
			// Concurrent first answers get the same tracker: Metrics.Tracker deduplicates.
			t = s.newConn()
			s.conn.Store(t)
		}
		t.Set(connstate.Connected)
	}
}

func (s *RemoteQueryService) GetState(ns driver.Namespace, key driver.PKey) (*driver.VaultValue, error) {
	if len(ns) == 0 || len(key) == 0 {
		return nil, ErrInvalidQueryInput
	}

	m := map[driver.Namespace][]driver.PKey{
		ns: {key},
	}

	logger.Debugf("QS GetState %v %v", ns, key)
	res, err := s.GetStates(m)
	if err != nil {
		return nil, errors.Wrap(err, "get states")
	}

	// ensure that ns exists
	if _, ok := res[ns]; !ok {
		return nil, nil
	}

	// key does not exist
	r, ok := res[ns][key]
	if !ok {
		return nil, nil
	}

	return &r, nil
}

func (s *RemoteQueryService) GetStates(m map[driver.Namespace][]driver.PKey) (map[driver.Namespace]map[driver.PKey]driver.VaultValue, error) {
	return s.query(m)
}

func (s *RemoteQueryService) GetTransactionStatus(txID string) (int32, error) {
	client, err := s.newClient()
	if err != nil {
		return 0, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), s.config.RequestTimeout)
	defer cancel()

	now := time.Now()
	res, err := client.GetTransactionStatus(ctx, &committerpb.TxStatusQuery{
		TxIds: []string{txID},
	})
	s.report(err)
	if err != nil {
		return 0, errors.Wrap(err, "query transaction status")
	}
	logger.Debugf("QS GetTransactionStatus: got response in %v", time.Since(now))
	if len(res.Statuses) == 0 {
		return 0, errors.Errorf("QS GetTransactionStatus: no statuses for tx %s", txID)
	}
	return int32(res.Statuses[0].Status), nil
}

// GetTransactionStatuses resolves the status of many transactions in one query,
// keyed by transaction ID. Duplicate input IDs are collapsed before querying.
// Unlike GetTransactionStatus, which returns an error for a transaction unknown
// to the committer, unknown transactions are omitted from the result so callers
// can treat a missing entry as "not final yet".
func (s *RemoteQueryService) GetTransactionStatuses(txIDs []string) (map[string]int32, error) {
	if len(txIDs) == 0 {
		return map[string]int32{}, nil
	}

	seen := make(map[string]struct{}, len(txIDs))
	unique := make([]string, 0, len(txIDs))
	for _, txID := range txIDs {
		if _, ok := seen[txID]; ok {
			continue
		}
		seen[txID] = struct{}{}
		unique = append(unique, txID)
	}

	client, err := s.newClient()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), s.config.RequestTimeout)
	defer cancel()

	now := time.Now()
	res, err := client.GetTransactionStatus(ctx, &committerpb.TxStatusQuery{TxIds: unique})
	s.report(err)
	if err != nil {
		return nil, errors.Wrap(err, "query transaction statuses")
	}
	logger.Debugf("QS GetTransactionStatuses: %d ids -> %d statuses in %v", len(unique), len(res.GetStatuses()), time.Since(now))

	out := make(map[string]int32, len(res.GetStatuses()))
	for _, st := range res.GetStatuses() {
		if ref := st.GetRef(); ref != nil {
			out[ref.GetTxId()] = int32(st.GetStatus())
		}
	}
	return out, nil
}

// GetConfigTransaction returns the envelope and version of the most recent config transaction
func (s *RemoteQueryService) GetConfigTransaction() (*ConfigTransactionInfo, error) {
	client, err := s.newClient()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), s.config.RequestTimeout)
	defer cancel()

	now := time.Now()
	res, err := client.GetConfigTransaction(ctx, &emptypb.Empty{})
	s.report(err)
	if err != nil {
		return nil, errors.Wrap(err, "get config transaction")
	}
	logger.Debugf("QS GetConfigTransaction: got response in %v", time.Since(now))

	// Unmarshal the envelope bytes into the protobuf structure
	envelope := &cb.Envelope{}
	if err := proto.Unmarshal(res.GetEnvelope(), envelope); err != nil {
		return nil, errors.Wrap(err, "unmarshal envelope")
	}

	return &ConfigTransactionInfo{
		Envelope: envelope,
		Version:  res.GetVersion(),
	}, nil
}

// GetNamespacePolicies returns the endorsement policies currently registered
// for every namespace, together with their version.
func (s *RemoteQueryService) GetNamespacePolicies() (*applicationpb.NamespacePolicies, error) {
	client, err := s.newClient()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), s.config.RequestTimeout)
	defer cancel()

	now := time.Now()
	res, err := client.GetNamespacePolicies(ctx, &emptypb.Empty{})
	s.report(err)
	if err != nil {
		return nil, errors.Wrap(err, "get namespace policies")
	}
	logger.Debugf("QS GetNamespacePolicies: got %d policies in %v", len(res.GetPolicies()), time.Since(now))

	return res, nil
}

func (s *RemoteQueryService) query(m map[driver.Namespace][]driver.PKey) (map[driver.Namespace]map[driver.PKey]driver.VaultValue, error) {
	logger.Debugf("QS GetState: query input %v", m)

	client, err := s.newClient()
	if err != nil {
		return nil, err
	}

	q, err := createQuery(m)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), s.config.RequestTimeout)
	defer cancel()

	now := time.Now()
	res, err := client.GetRows(ctx, q)
	s.report(err)
	if err != nil {
		logger.Warnf("QS GetState: error calling getRows: %v", err)
		return nil, errors.Wrap(err, "query get rows")
	}
	logger.Debugf("QS GetState: got response in %v", time.Since(now))

	result := createResult(res)
	logger.Debugf("QS GetState: res: %v", result)
	return result, nil
}

// createQuery converts an input map into a `protoqueryservice.Query`.
// It returns a `ErrInvalidQueryInput` error if the input is invalid, in particular, if the input is empty
// of a namespace does not contain any keys.
func createQuery(m map[driver.Namespace][]driver.PKey) (*committerpb.Query, error) {
	if len(m) == 0 {
		return nil, ErrInvalidQueryInput
	}

	namespaces := make([]*committerpb.QueryNamespace, 0, len(m))
	for nsName, keys := range m {
		if len(nsName) == 0 {
			return nil, ErrInvalidQueryInput
		}

		// if there is a namespace defined but not keys, we through an error
		if len(keys) == 0 {
			return nil, ErrInvalidQueryInput
		}

		nsKeys := make([][]byte, 0, len(keys))
		for _, k := range keys {
			if len(k) == 0 {
				return nil, ErrInvalidQueryInput
			}
			nsKeys = append(nsKeys, []byte(k))
		}

		namespaces = append(namespaces, &committerpb.QueryNamespace{NsId: nsName, Keys: nsKeys})
	}

	return &committerpb.Query{Namespaces: namespaces}, nil
}

// createResult converts a response (`protoqueryservice.Rows`) into a 2-dim map data structure.
// If the response (rows) is empty, createResult returns an empty map.
func createResult(res *committerpb.Rows) map[driver.Namespace]map[driver.PKey]driver.VaultValue {
	result := make(map[driver.Namespace]map[driver.PKey]driver.VaultValue, len(res.GetNamespaces()))

	for _, r := range res.GetNamespaces() {
		result[r.GetNsId()] = make(map[driver.PKey]driver.VaultValue, len(r.GetRows()))
		for _, item := range r.GetRows() {
			key := driver.PKey(item.GetKey())
			result[r.GetNsId()][key] = driver.VaultValue{
				Raw:     item.GetValue(),
				Version: protowire.AppendVarint(nil, item.GetVersion()),
			}
		}
	}

	return result
}
