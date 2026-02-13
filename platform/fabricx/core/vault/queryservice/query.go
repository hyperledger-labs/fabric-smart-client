/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package queryservice

import (
	"context"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger/fabric-x-committer/api/protoqueryservice"
	"google.golang.org/protobuf/encoding/protowire"
)

var (
	logger = logging.MustGetLogger()

	ErrInvalidQueryInput = errors.New("invalid query input")
)

type RemoteQueryService struct {
	client protoqueryservice.QueryServiceClient
	config *Config
}

func NewRemoteQueryService(config *Config, client protoqueryservice.QueryServiceClient) *RemoteQueryService {
	return &RemoteQueryService{
		client: client,
		config: config,
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
	ctx, cancel := context.WithTimeout(context.Background(), s.config.QueryTimeout)
	defer cancel()

	now := time.Now()
	res, err := s.client.GetTransactionStatus(ctx, &protoqueryservice.TxStatusQuery{
		TxIds: []string{txID},
	})
	if err != nil {
		return 0, errors.Wrap(err, "query transaction status")
	}
	logger.Debugf("QS GetTransactionStatus: got response in %v", time.Since(now))
	if len(res.Statuses) == 0 {
		return 0, errors.Errorf("QS GetTransactionStatus: no statuses for tx %s", txID)
	}
	return int32(res.Statuses[0].StatusWithHeight.Code), nil
}

func (s *RemoteQueryService) query(m map[driver.Namespace][]driver.PKey) (map[driver.Namespace]map[driver.PKey]driver.VaultValue, error) {
	logger.Debugf("QS GetState: query input %v", m)

	q, err := createQuery(m)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), s.config.QueryTimeout)
	defer cancel()

	now := time.Now()
	res, err := s.client.GetRows(ctx, q)
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
func createQuery(m map[driver.Namespace][]driver.PKey) (*protoqueryservice.Query, error) {
	if len(m) == 0 {
		return nil, ErrInvalidQueryInput
	}

	namespaces := make([]*protoqueryservice.QueryNamespace, 0, len(m))
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

		namespaces = append(namespaces, &protoqueryservice.QueryNamespace{NsId: nsName, Keys: nsKeys})
	}

	return &protoqueryservice.Query{Namespaces: namespaces}, nil
}

// createResult converts a response (`protoqueryservice.Rows`) into a 2-dim map data structure.
// If the response (rows) is empty, createResult returns an empty map.
func createResult(res *protoqueryservice.Rows) map[driver.Namespace]map[driver.PKey]driver.VaultValue {
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
