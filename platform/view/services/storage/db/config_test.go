package db

import (
	"os"
	"path"
	"testing"

	config2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/core/config"
	mem "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/memory"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
)

var testMatrix = map[PersistenceName]types.GomegaMatcher{
	"memtest": HaveField("Type", mem.MemoryPersistence),
	"": And(
		HaveField("Type", sql.SQLPersistence),
		HaveField("Opts", And(
			HaveField("Driver", sql.SQLite),
			HaveField("MaxOpenConns", 20),
		)),
	),
	"notexisting": HaveField("Type", mem.MemoryPersistence),
}

func TestPersistenceConfig(t *testing.T) {
	RegisterTestingT(t)

	confPath := path.Join(os.Getenv("GOPATH"), "src", "github.com", "hyperledger-labs", "fabric-smart-client", "platform", "view", "services", "storage", "db")
	provider, err := config2.NewProvider(confPath)
	Expect(err).ToNot(HaveOccurred())
	perCfg := newPersistenceConfig(provider)

	for name, matchCondition := range testMatrix {
		cfg, err := perCfg.Get(name)
		Expect(err).ToNot(HaveOccurred())
		Expect(cfg).To(matchCondition)
	}
}
