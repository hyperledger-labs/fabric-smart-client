# View API

The `View API` is a set of abstractions that enable the implementation of Business Processes as interactive protocols in an
implementation and blockchain independent way.

The View API consists of the following abstractions:

## Registry

The View API provides a registry that allows to register `initiators`, using factories, and
`responders`.

Here is an example of how to use the registry:

```go
package main

import (
	"github.com/hyperledger-labs/fabric-smart-client/integration/fsc/pingpong"
	fscnode "github.com/hyperledger-labs/fabric-smart-client/node"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view"
)

func main() {
	node := fscnode.New()
	node.Execute(func() error {
		registry := view.GetRegistry(node)
		if err := registry.RegisterFactory("init", &pingpong.InitiatorViewFactory{}); err != nil {
			return err
		}
		return nil
	})
}
```

