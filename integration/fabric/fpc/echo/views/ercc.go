package views

import (
	"encoding/json"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/fpc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/assert"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type ListProvisionedEnclaves struct {
	CID string
}

type ListProvisionedEnclavesView struct {
	*ListProvisionedEnclaves
}

func (l *ListProvisionedEnclavesView) Call(context view.Context) (interface{}, error) {
	pEnclaves, err := fpc.GetProvider(context).EnclaveRegistry().ListProvisionedEnclaves(l.CID)
	assert.NoError(err, "failed getting list of provisioned enclaves for [%s]", l.CID)

	return pEnclaves, nil
}

type ListProvisionedEnclavesViewFactory struct{}

func (l *ListProvisionedEnclavesViewFactory) NewView(in []byte) (view.View, error) {
	f := &ListProvisionedEnclavesView{}
	assert.NoError(json.Unmarshal(in, &f.ListProvisionedEnclaves))
	return f, nil
}
