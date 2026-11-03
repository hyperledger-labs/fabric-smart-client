/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package start

import (
	"context"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host/websocket"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host/websocket/ws"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/preamble"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/endpoint"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/id"
)

// Configured gives a command the node's configuration without installing any SDK.
type Configured interface {
	Config() (driver.ConfigService, error)
}

// peerTimeout bounds the dial and preamble read of one peer.
const peerTimeout = 10 * time.Second

// peer is a configured peer to check: its resolver name, P2P peer ID and P2P address.
type peer struct {
	name, id, address string
}

// readPreamble returns the encoded preamble of peer id at address; see ws.ReadPreamble.
type readPreamble func(ctx context.Context, id, address string) ([]byte, error)

func peersCmd(c Configured) *cobra.Command {
	var check bool
	cmd := &cobra.Command{
		Use:   "peers",
		Short: "Checks that this binary can talk to the node's configured peers.",
		Long: `With --check, dials every peer in fsc.endpoint.resolvers with the node's P2P identity,
reads the preamble the peer sends, disconnects, and reports per component whether this
binary's range of wire format versions overlaps the peer's. It opens no session, starts no
view, opens no database and changes nothing. Only websocket P2P is supported.

Exit codes: 0 every peer is compatible, 11 a peer's framing epoch differs or a component's
ranges do not overlap, 1 a peer could not be reached or the command failed, 2 invalid arguments.`,
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return usageError(cmd, errors.Errorf("unexpected arguments %q", args))
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !check {
				return usageError(cmd, errors.New("--check is required"))
			}
			cmd.SilenceUsage = true
			cfg, err := c.Config()
			if err != nil {
				return &ExitError{Code: ExitFailed, Err: err}
			}
			peers, dial, err := configuredPeers(cfg)
			if err != nil {
				return &ExitError{Code: ExitFailed, Err: err}
			}
			if code := checkPeers(cmd.Context(), cmd.OutOrStdout(), peers, dial); code != 0 {
				return &ExitError{Code: code, Err: errors.Errorf("peer check failed with exit code %d", code)}
			}
			return nil
		},
	}
	cmd.SetFlagErrorFunc(usageError)
	cmd.Flags().BoolVar(&check, "check", false, "dial every configured peer and compare its version ranges with this binary's")
	return cmd
}

// resolver is the part of an fsc.endpoint.resolvers entry that locates a peer.
type resolver struct {
	Name     string `yaml:"name"`
	Identity struct {
		Path string `yaml:"path"`
	} `yaml:"identity"`
	Addresses map[string]string `yaml:"addresses"`
}

// configuredPeers returns the peers in fsc.endpoint.resolvers that the node dials: the first
// entry of each name that has a P2P address, other than the node itself. It refuses the entries
// the node's resolver loader refuses. The dialer authenticates as the node. Like the node's websocket
// host, the dialer trusts the configured root CAs and every resolver's identity certificate.
func configuredPeers(cfg driver.ConfigService) ([]peer, readPreamble, error) {
	if t := cfg.GetString("fsc.p2p.type"); !strings.EqualFold(t, websocket.P2PCommunicationType) {
		return nil, nil, errors.Errorf("fsc.p2p.type [%s] is not supported, only [%s]", t, websocket.P2PCommunicationType)
	}
	wsConfig, err := websocket.NewConfig(cfg)
	if err != nil {
		return nil, nil, err
	}
	tlsConfig, err := wsConfig.ClientTLSConfig()
	if err != nil {
		return nil, nil, err
	}
	if tlsConfig == nil {
		return nil, nil, errors.New("websocket p2p requires TLS")
	}
	selfRaw, err := id.LoadIdentity(wsConfig.CertPath())
	if err != nil {
		return nil, nil, errors.WithMessage(err, "failed loading the node's p2p identity")
	}
	self, err := pkID(selfRaw)
	if err != nil {
		return nil, nil, errors.WithMessage(err, "failed loading the node's p2p identity")
	}
	tlsConfig = tlsConfig.Clone()
	tlsConfig.RootCAs = tlsConfig.RootCAs.Clone()

	var resolvers []resolver
	if err := cfg.UnmarshalKey("fsc.endpoint.resolvers", &resolvers); err != nil {
		return nil, nil, errors.Wrap(err, "failed loading fsc.endpoint.resolvers")
	}
	var peers []peer
	seen := map[string]bool{}
	for i, r := range resolvers {
		if r.Name == "" {
			return nil, nil, errors.Errorf("fsc.endpoint.resolvers[%d]: name is required", i)
		}
		if r.Identity.Path == "" {
			return nil, nil, errors.Errorf("fsc.endpoint.resolvers[%d] [%s]: identity.path is required", i, r.Name)
		}
		raw, err := id.LoadIdentity(cfg.TranslatePath(r.Identity.Path))
		if err != nil {
			return nil, nil, errors.WithMessagef(err, "fsc.endpoint.resolvers[%d] [%s]", i, r.Name)
		}
		addresses, err := endpoint.ConvertAddresses(r.Addresses)
		if err != nil {
			return nil, nil, errors.WithMessagef(err, "fsc.endpoint.resolvers[%d] [%s]", i, r.Name)
		}
		tlsConfig.RootCAs.AppendCertsFromPEM(raw)
		// The node keeps the addresses of the first entry of a name and only binds the
		// identities of later ones to it, so later entries are never dialed.
		first := !seen[r.Name]
		seen[r.Name] = true
		address := addresses[endpoint.P2PPort]
		if !first || address == "" {
			continue
		}
		pid, err := pkID(raw)
		if err != nil {
			return nil, nil, errors.WithMessagef(err, "fsc.endpoint.resolvers[%d] [%s]", i, r.Name)
		}
		if pid == self {
			continue
		}
		peers = append(peers, peer{name: r.Name, id: pid, address: address})
	}
	dial := func(ctx context.Context, peerID, address string) ([]byte, error) {
		c := tlsConfig.Clone()
		if host, _, err := net.SplitHostPort(address); err == nil {
			c.ServerName = host
		} else {
			c.ServerName = address
		}
		return ws.ReadPreamble(ctx, peerID, address, c)
	}
	return peers, dial, nil
}

// pkID returns the P2P peer ID of the identity certificate raw, as the websocket host derives it.
func pkID(raw []byte) (string, error) {
	pk, err := (&comm.PKExtractor{}).ExtractPublicKey(raw)
	if err != nil {
		return "", err
	}
	pid, err := websocket.PKIDSynthesizer{}.PublicKeyID(pk)
	return string(pid), err
}

// checkPeers reads each peer's preamble with dial, writes one line per peer and component to
// w, and returns the exit code: ExitFailed if a peer could not be reached, otherwise
// ExitIncompatible if one is incompatible, otherwise 0. A peer of a release without preambles,
// or a component that only one side announces, is reported but not counted as incompatible:
// the node accepts such peers.
func checkPeers(ctx context.Context, w io.Writer, peers []peer, dial readPreamble) int {
	if len(peers) == 0 {
		_, _ = fmt.Fprintln(w, "no peers configured in fsc.endpoint.resolvers")
	}
	local := preamble.Local()
	unreachable, incompatible := false, false
	for _, p := range peers {
		peerCtx, cancel := context.WithTimeout(ctx, peerTimeout)
		raw, err := dial(peerCtx, p.id, p.address)
		cancel()
		if errors.Is(err, ws.ErrBadPreamble) {
			incompatible = true
			_, _ = fmt.Fprintf(w, "peer [%s] at [%s]: incompatible: %v\n", p.name, p.address, err)
			continue
		}
		if err != nil {
			unreachable = true
			_, _ = fmt.Fprintf(w, "peer [%s] at [%s]: unreachable: %v\n", p.name, p.address, err)
			continue
		}
		if raw == nil {
			_, _ = fmt.Fprintf(w, "peer [%s] at [%s]: no preamble, it runs an FSC release without version ranges\n", p.name, p.address)
			continue
		}
		remote, err := preamble.Accept(raw)
		if err != nil {
			incompatible = true
			_, _ = fmt.Fprintf(w, "peer [%s] at [%s]: incompatible: %v\n", p.name, p.address, err)
			continue
		}
		_, _ = fmt.Fprintf(w, "peer [%s] at [%s]: FSC [%s] framing epoch %d\n", p.name, p.address, remote.GetFscVersion(), remote.GetFramingEpoch())
		for _, l := range local.GetRanges() {
			line, ok := compareRange(l, remote.GetRanges())
			incompatible = incompatible || !ok
			_, _ = fmt.Fprintf(w, "  %s\n", line)
		}
	}
	switch {
	case unreachable:
		return ExitFailed
	case incompatible:
		return ExitIncompatible
	default:
		return 0
	}
}

// compareRange describes how the local range l compares with the peer's range of the same
// component, and reports false if both exist and do not overlap. Two overlapping ranges use the
// highest version in both.
func compareRange(l *preamble.ComponentRange, remote []*preamble.ComponentRange) (string, bool) {
	for _, r := range remote {
		if r.GetComponent() != l.GetComponent() {
			continue
		}
		lo, hi := max(l.GetMin(), r.GetMin()), min(l.GetMax(), r.GetMax())
		desc := fmt.Sprintf("%s: local [%d,%d] peer [%d,%d]", l.GetComponent(), l.GetMin(), l.GetMax(), r.GetMin(), r.GetMax())
		if lo > hi {
			return desc + ": no overlap", false
		}
		return fmt.Sprintf("%s: uses %d", desc, hi), true
	}
	return fmt.Sprintf("%s: local [%d,%d], not announced by the peer", l.GetComponent(), l.GetMin(), l.GetMax()), true
}
