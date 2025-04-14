package db

import (
	"fmt"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/lazy"
	mem "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/memory"
)

type PersistenceName string

const DefaultPersistence PersistenceName = "default"

type persistenceConfig struct {
	provider lazy.Provider[PersistenceName, *Config]
}

func newPersistenceConfig(cp config) *persistenceConfig {
	return &persistenceConfig{provider: lazy.NewProvider(func(key PersistenceName) (*Config, error) {
		return getConfig(cp, fmt.Sprintf("fsc.persistences.%s", key))
	})}
}

func (c *persistenceConfig) Get(name PersistenceName) (*Config, error) {
	if len(name) == 0 {
		name = DefaultPersistence
	}
	return c.provider.Get(name)
}

func getConfig(cp config, configKey string) (*Config, error) {
	var cfg Config
	if err := cp.UnmarshalKey(configKey, &cfg); err != nil || !supportedStores.Contains(cfg.Type) || cfg.Type == mem.MemoryPersistence {
		logger.Warnf("Persistence type [%s]. Supported: [%v]. Error: %v", cfg.Type, supportedStores, err)
		return &Config{
			Type: mem.MemoryPersistence,
			Opts: memOpts,
		}, nil
	}

	if len(cfg.Opts.Driver) == 0 {
		return nil, notSetError("driver")
	}
	if len(cfg.Opts.DataSource) == 0 {
		return nil, notSetError("dataSource")
	}
	if cfg.Opts.MaxIdleConns == nil {
		cfg.Opts.MaxIdleConns = defaultMaxIdleConns
	}
	if cfg.Opts.MaxIdleTime == nil {
		cfg.Opts.MaxIdleTime = defaultMaxIdleTime
	}
	return &cfg, nil
}
