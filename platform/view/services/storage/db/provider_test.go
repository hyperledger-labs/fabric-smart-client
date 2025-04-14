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
)

func TestConfigProvider(t *testing.T) {
	RegisterTestingT(t)

	confPath := path.Join(os.Getenv("GOPATH"), "src", "github.com", "hyperledger-labs", "fabric-smart-client", "platform", "view", "services", "storage", "db")
	provider, err := config2.NewProvider(confPath)
	Expect(err).ToNot(HaveOccurred())
	cp := db.NewConfigProvider(provider)
	persistence := db.PersistenceName(provider.GetString("fsc.kvs.persistence"))
	c, err := cp.GetConfig(persistence, "")
	Expect(err).ToNot(HaveOccurred())
	Expect(c.DriverType()).To(Equal(mem.MemoryPersistence))
	Expect(c.TableName()).To(HaveLen(16))
	Expect(c.DbOpts().Driver()).To(Equal(sql.SQLite))
}
