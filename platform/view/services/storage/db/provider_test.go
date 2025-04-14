package db_test

import (
	"os"
	"path"
	"testing"

	config2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/core/config"
	mem "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/memory"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/db"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
)

var testMatrix = map[string]types.GomegaMatcher{
	"fsc.kvs.persistence": And(
		HaveField("DriverType()", mem.MemoryPersistence),
		HaveField("TableName()", HaveLen(16)),
		HaveField("DbOpts()", And(
			HaveField("Driver()", sql.SQLite),
		)),
	),
}

func TestConfigProvider(t *testing.T) {
	RegisterTestingT(t)

	confPath := path.Join(os.Getenv("GOPATH"), "src", "github.com", "hyperledger-labs", "fabric-smart-client", "platform", "view", "services", "storage", "db")
	provider, err := config2.NewProvider(confPath)
	Expect(err).ToNot(HaveOccurred())
	cp := db.NewConfigProvider(provider)

	for configKey, matchCondition := range testMatrix {
		persistence := db.PersistenceName(provider.GetString(configKey))
		c, err := cp.GetConfig(persistence, "")

		Expect(err).ToNot(HaveOccurred())
		Expect(c).To(matchCondition)
	}
}
