/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fsc

import (
	"bytes"
	"strings"
	"testing"
	"text/template"

	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"

	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/api"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc/node"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
)

// templateData stands in for the Platform as the data the core template is executed with.
type templateData struct {
	P2PCommunicationType string
}

func (templateData) NodeLocalCertPath(*node.Peer) string       { return "cert.pem" }
func (templateData) NodeLocalPrivateKeyPath(*node.Peer) string { return "key.pem" }
func (templateData) NodeLocalTLSDir(*node.Peer) string         { return "tls" }
func (templateData) NodeDir(*node.Replica) string              { return "node" }
func (templateData) NodePort(*node.Replica, api.PortName) int  { return 1234 }
func (templateData) ClientAuthRequired() bool                  { return false }
func (templateData) BootstrapNode(*node.Peer) string           { return "" }

func renderCoreTemplate(t *testing.T, tmpl string, topology *Topology) string {
	t.Helper()

	peer := &node.Peer{Node: node.NewNode("alice"), Name: "alice", Organization: "org"}
	replica := node.NewReplica(peer, "alice.0")
	funcs := template.FuncMap{
		"Replica":         func() *node.Replica { return replica },
		"Peer":            func() *node.Peer { return peer },
		"Topology":        func() *Topology { return topology },
		"Extensions":      func() []string { return nil },
		"Persistences":    func() map[driver.PersistenceName]node.PersistenceOpts { return nil },
		"Resolvers":       func() []*Resolver { return nil },
		"WebEnabled":      func() bool { return false },
		"TracingEndpoint": func() string { return "0.0.0.0:4317" },
		"SamplingRatio":   func() float64 { return 0 },
	}
	tpl, err := template.New("core").Funcs(funcs).Parse(tmpl)
	require.NoError(t, err)

	var buf bytes.Buffer
	require.NoError(t, tpl.Execute(&buf, templateData{P2PCommunicationType: WebSocket}))
	return buf.String()
}

func p2pSection(t *testing.T, core string) map[string]any {
	t.Helper()

	var cfg struct {
		FSC struct {
			P2P map[string]any `yaml:"p2p"`
		} `yaml:"fsc"`
	}
	require.NoError(t, yaml.Unmarshal([]byte(core), &cfg))
	return cfg.FSC.P2P
}

func TestCoreTemplate_P2PMessageSizeLimits(t *testing.T) {
	t.Parallel()

	// The template as it renders when the limits are not part of it.
	const block = "    {{- if gt Topology.P2P.MaxRecvMsgSize 0 }}\n" +
		"    maxRecvMsgSize: {{ Topology.P2P.MaxRecvMsgSize }}\n" +
		"    {{- end }}\n" +
		"    {{- if gt Topology.P2P.MaxSendMsgSize 0 }}\n" +
		"    maxSendMsgSize: {{ Topology.P2P.MaxSendMsgSize }}\n" +
		"    {{- end }}\n"
	require.Contains(t, node.CoreTemplate, block)
	withoutLimits := strings.Replace(node.CoreTemplate, block, "", 1)

	tests := []struct {
		name     string
		set      func(*Topology)
		wantRecv any
		wantSend any
	}{
		{name: "nothing set", set: func(*Topology) {}},
		{name: "both set", set: func(tp *Topology) { tp.SetP2PMessageSizeLimits(20<<20, 30<<20) }, wantRecv: 20 << 20, wantSend: 30 << 20},
		{name: "only receive set", set: func(tp *Topology) { tp.SetP2PMessageSizeLimits(20<<20, 0) }, wantRecv: 20 << 20},
		{name: "only send set", set: func(tp *Topology) { tp.SetP2PMessageSizeLimits(0, 30<<20) }, wantSend: 30 << 20},
		{name: "negative values", set: func(tp *Topology) { tp.SetP2PMessageSizeLimits(-1, -1) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			topology := NewTopology()
			tt.set(topology)
			core := renderCoreTemplate(t, node.CoreTemplate, topology)

			p2p := p2pSection(t, core)
			require.Equal(t, tt.wantRecv, p2p["maxRecvMsgSize"])
			require.Equal(t, tt.wantSend, p2p["maxSendMsgSize"])
			require.Contains(t, p2p, "opts", "the keys must not displace the rest of fsc.p2p")
			require.Contains(t, p2p, "listenAddress")

			if tt.wantRecv == nil && tt.wantSend == nil {
				require.Equal(t, renderCoreTemplate(t, withoutLimits, topology), core,
					"without limits the rendered core.yaml is unchanged")
			}
		})
	}
}

func TestTopology_SetP2PMessageSizeLimits(t *testing.T) {
	t.Parallel()

	topology := NewTopology()
	require.Equal(t, P2P{}, topology.P2P)

	topology.SetP2PMessageSizeLimits(1, 2)
	require.Equal(t, P2P{MaxRecvMsgSize: 1, MaxSendMsgSize: 2}, topology.P2P)

	topology.SetP2PMessageSizeLimits(-5, 0)
	require.Equal(t, P2P{}, topology.P2P)
}
