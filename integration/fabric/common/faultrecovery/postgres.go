/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package faultrecovery

import (
	"os"
	"os/exec"
	"slices"
	"strings"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"go.yaml.in/yaml/v3"

	"github.com/hyperledger-labs/fabric-smart-client/integration"
	nwonode "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc/node"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/postgres"
)

// The helpers below drive Postgres through the docker CLI, which the integration
// suites already require, and fail the spec on any error.

// StartStandby starts the container standby, from the image of primary, as a
// streaming-replication standby of the nwo-started Postgres container primary and
// returns its port on 127.0.0.1. A standby left over from an aborted run is replaced,
// and the standby is removed when the spec ends.
//
// The image already runs with wal_level=replica; StartStandby only lets replication
// connections in from the Docker bridge, which the standby uses to reach the primary,
// so the standby keeps running when the primary is killed.
func StartStandby(primary, standby string) string {
	docker("exec", primary, "sh", "-c", `echo "host replication all all trust" >> "$PGDATA/pg_hba.conf"`)
	psql(primary, "SELECT pg_reload_conf()")
	ip := docker("inspect", "-f", "{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}", primary)
	gomega.Expect(ip).NotTo(gomega.BeEmpty(), "container [%s] has no bridge IP address", primary)
	image := docker("inspect", "-f", "{{.Config.Image}}", primary)
	user := psql(primary, "SELECT current_user")
	db := psql(primary, "SELECT current_database()")

	removeStandby := func() { _ = exec.Command("docker", "rm", "-f", "-v", standby).Run() }
	removeStandby()
	ginkgo.DeferCleanup(removeStandby)
	docker("run", "-d", "--name", standby, "-p", "127.0.0.1::5432",
		"-e", "PGDATA=/var/lib/postgresql/standby", "-e", "POSTGRES_USER="+user, "-e", "POSTGRES_DB="+db, "--entrypoint", "sh", image, "-c",
		`mkdir -p "$PGDATA" && chown postgres "$PGDATA" && chmod 700 "$PGDATA" && `+
			`su-exec postgres pg_basebackup -h `+ip+` -U `+user+` -D "$PGDATA" -R -X stream && `+
			`exec su-exec postgres postgres`)
	gomega.Eventually(func() error {
		return exec.Command("docker", "exec", standby, "pg_isready", "-q").Run()
	}, time.Minute, time.Second).Should(gomega.Succeed(), func() string {
		logs, _ := exec.Command("docker", "logs", "--tail", "20", standby).CombinedOutput()
		return "standby [" + standby + "] did not start:\n" + string(logs)
	})

	_, port, _ := strings.Cut(docker("port", standby, "5432"), ":")
	return port
}

// WaitReplicated returns once standby has replayed everything primary has written so far.
func WaitReplicated(primary, standby string) {
	lsn := psql(primary, "SELECT pg_current_wal_lsn()")
	gomega.Eventually(func() (string, error) {
		out, err := exec.Command("docker", "exec", standby, "sh", "-c", psqlCmd,
			"SELECT pg_last_wal_replay_lsn() >= '"+lsn+"'::pg_lsn").Output()
		return strings.TrimSpace(string(out)), err
	}, time.Minute, 500*time.Millisecond).Should(gomega.Equal("t"), "standby [%s] did not catch up", standby)
}

// KillPostgres kills the Postgres container name without a clean shutdown.
func KillPostgres(name string) { docker("kill", name) }

// Promote promotes standby to a primary that accepts writes. pg_promote waits until
// the promotion has completed.
func Promote(standby string) {
	gomega.Expect(psql(standby, "SELECT pg_promote()")).To(gomega.Equal("t"))
}

// DataSource returns the data source nwo configured for node's default persistence.
func DataSource(ii *integration.Infrastructure, node string) *postgres.ContainerConfig {
	opts := replica(ii, node).Options.GetPostgresPersistences()["default"]
	gomega.Expect(opts).NotTo(gomega.BeNil(), "FSC node [%s] has no Postgres default persistence", node)
	c, err := postgres.ConfigFromDataSource(opts.DataSource)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	return c
}

// SetDataSource sets the dataSource of the default persistence in the core.yaml of the
// stopped FSC node, which reads it on its next start.
func SetDataSource(ii *integration.Infrastructure, node, dataSource string) {
	path := ii.FscPlatform.NodeConfigPath(replica(ii, node))

	raw, err := os.ReadFile(path)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	var cfg map[string]any
	gomega.Expect(yaml.Unmarshal(raw, &cfg)).To(gomega.Succeed())

	at(cfg, "fsc", "persistences", "default", "opts")["dataSource"] = dataSource

	raw, err = yaml.Marshal(cfg)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	gomega.Expect(os.WriteFile(path, raw, 0o600)).To(gomega.Succeed())
}

// replica returns the FSC node named node.
func replica(ii *integration.Infrastructure, node string) *nwonode.Replica {
	i := slices.IndexFunc(ii.FscPlatform.Peers, func(p *nwonode.Replica) bool { return p.Name == node })
	gomega.ExpectWithOffset(1, i).NotTo(gomega.Equal(-1), "no FSC node named [%s]", node)
	return ii.FscPlatform.Peers[i]
}

// at returns the map at the given keys below m.
func at(m map[string]any, keys ...string) map[string]any {
	for _, k := range keys {
		next, ok := m[k].(map[string]any)
		gomega.ExpectWithOffset(1, ok).To(gomega.BeTrue(), "core.yaml has no map at [%s]", k)
		m = next
	}
	return m
}

// psqlCmd runs the query passed as $0 as the container's Postgres superuser and prints
// the bare result.
const psqlCmd = `psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -tAc "$0"`

// psql runs query in container with psqlCmd.
func psql(container, query string) string {
	return docker("exec", container, "sh", "-c", psqlCmd, query)
}

func docker(args ...string) string {
	out, err := exec.Command("docker", args...).CombinedOutput()
	gomega.ExpectWithOffset(2, err).NotTo(gomega.HaveOccurred(), "docker %v: %s", args, out)
	return strings.TrimSpace(string(out))
}
