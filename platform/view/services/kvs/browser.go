/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package kvs

import (
	"encoding/json"
	"html/template"
	"net/http"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/hash"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/kvs/browser"
	"github.com/pkg/errors"
)

type Server interface {
	RegisterHandler(s string, handler http.Handler, secure bool)
}

type Entry struct {
	Key   string
	Value string
}

type errorResponse struct {
	Error string `json:"Error"`
}

type Browser struct {
	Server Server
	KVS    *KVS
}

func RegisterWebHandler(server Server, kvs *KVS) *Browser {
	b := &Browser{Server: server, KVS: kvs}
	b.Server.RegisterHandler("/kvs", b, true)
	return b
}

func (b *Browser) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	it, err := b.KVS.GetRawIteratorByPartialCompositeID("", nil)
	if err != nil {
		b.sendResponse(writer, http.StatusBadRequest, errors.WithMessage(err, "failed to get iterator over Vault"))
	}
	defer it.Close()
	var list []*Entry
	for i := 0; i < 100; i++ {
		n, err := it.Next()
		if err != nil {
			break
		}
		if n == nil {
			break
		}
		list = append(list, &Entry{
			Key:   n.Key,
			Value: hash.Hashable(n.Raw).String(),
		})
	}
	logger.Infof("retrieved [%d] entries", len(list))

	t, err := template.New("peer").Funcs(template.FuncMap{
		"Items": func() []*Entry { return list },
	}).Parse(browser.TableTemplate)
	if err != nil {
		b.sendResponse(writer, http.StatusBadRequest, errors.WithMessage(err, "failed to parse template"))
	}

	if err := t.Execute(writer, nil); err != nil {
		b.sendResponse(writer, http.StatusBadRequest, errors.WithMessage(err, "failed to process template"))
	}
}

func (b *Browser) sendResponse(resp http.ResponseWriter, code int, payload interface{}) {
	if err, ok := payload.(error); ok {
		payload = &errorResponse{Error: err.Error()}
	}
	js, err := json.Marshal(payload)
	if err != nil {
		logger := flogging.MustGetLogger("operations.runner")
		logger.Errorw("failed to encode payload", "error", err)
		resp.WriteHeader(http.StatusInternalServerError)
		return
	}
	resp.Header().Set("Content-Type", "application/json")
	resp.WriteHeader(code)
	resp.Write(js)
}
