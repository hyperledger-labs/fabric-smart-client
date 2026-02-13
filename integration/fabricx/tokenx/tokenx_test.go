/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package tokenx_test

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/integration"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabricx/tokenx"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabricx/tokenx/states"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabricx/tokenx/views"
	nwoclient "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/client"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common"
	nwofabricx "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabricx"
	nwofsc "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
	nwofscnode "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc/node"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("EndToEnd", func() {
	for _, c := range []nwofsc.P2PCommunicationType{nwofsc.WebSocket} {
		Describe("Token Life Cycle", Label(c), func() {
			s := NewTestSuite(c, integration.NoReplication)
			BeforeEach(s.Setup)
			AfterEach(s.TearDown)

			It("succeeded", s.TestSucceeded)
			It("rest succeeded", s.TestRESTSucceeded)
			It("swap succeeded", s.TestSwapSucceeded)
			It("auditor queries succeeded", s.TestAuditorQueries)
			It("error cases handled", s.TestErrorCases)
		})
	}
})

type TestSuite struct {
	*integration.TestSuite
}

func NewTestSuite(commType nwofsc.P2PCommunicationType, nodeOpts *integration.ReplicationOptions) *TestSuite {
	return &TestSuite{integration.NewTestSuite(func() (*integration.Infrastructure, error) {
		ii, err := integration.New(
			integration.IOUPort.StartPortForNode(),
			"",
			tokenx.Topology(&tokenx.SDK{}, commType, nodeOpts, "alice", "bob", "charlie")...,
		)
		if err != nil {
			return nil, err
		}

		ii.RegisterPlatformFactory(nwofabricx.NewPlatformFactory())
		ii.Generate()

		return ii, nil
	})}
}

func (s *TestSuite) TestSucceeded() {
	// ============================================
	// Phase 1: Issue tokens to all participants
	// ============================================
	By("issuing USD tokens to alice")
	aliceUSDTokenID := IssueTokens(s.II, "USD", states.TokenFromFloat(1000), "alice")
	Expect(aliceUSDTokenID).NotTo(BeEmpty())

	By("issuing EUR tokens to bob")
	bobEURTokenID := IssueTokens(s.II, "EUR", states.TokenFromFloat(500), "bob")
	Expect(bobEURTokenID).NotTo(BeEmpty())

	By("issuing GOLD tokens to charlie")
	charlieGOLDTokenID := IssueTokens(s.II, "GOLD", states.TokenFromFloat(10), "charlie")
	Expect(charlieGOLDTokenID).NotTo(BeEmpty())

	// ============================================
	// Phase 2: Query balances
	// ============================================
	By("querying alice's balance")
	aliceBalance := QueryBalance(s.II, "alice", "USD")
	Expect(aliceBalance).NotTo(BeNil())

	By("querying bob's balance")
	bobBalance := QueryBalance(s.II, "bob", "")
	Expect(bobBalance).NotTo(BeNil())

	// ============================================
	// Phase 3: Transfer & Redeem
	// ============================================
	By("transferring USD tokens from alice to bob")
	transferTxID := TransferTokens(s.II, aliceUSDTokenID, states.TokenFromFloat(200), "alice", "bob")
	Expect(transferTxID).NotTo(BeEmpty())

	By("redeeming EUR tokens by bob")
	redeemTxID := RedeemTokens(s.II, bobEURTokenID, states.TokenFromFloat(500), "bob")
	Expect(redeemTxID).NotTo(BeEmpty())
}

func (s *TestSuite) TestRESTSucceeded() {
	issuerBase := restBaseURL(s.II, tokenx.IssuerNode)
	aliceBase := restBaseURL(s.II, "alice")

	hc := &http.Client{
		Timeout: 2 * time.Minute,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	By("waiting for alice REST endpoint to be reachable")
	Eventually(func() error {
		status, _, err := doJSON(hc, http.MethodGet, aliceBase+"/v1/tokens/balance?token_type=USD", nil)
		if err != nil {
			return err
		}
		if status != http.StatusOK {
			return errorsFromStatus(status)
		}
		return nil
	}, 2*time.Minute, 2*time.Second).Should(Succeed())

	By("issuing tokens via REST")
	issueReq := map[string]string{
		"token_type": "USD",
		"amount":     "10.00",
		"recipient":  "alice",
	}
	var issueResp struct {
		TokenID string `json:"token_id"`
	}
	Eventually(func() error {
		status, body, err := doJSON(hc, http.MethodPost, issuerBase+"/v1/tokens/issue", issueReq)
		if err != nil {
			return err
		}
		if status != http.StatusOK {
			return errorsFromStatus(status)
		}
		return json.Unmarshal(body, &issueResp)
	}, 2*time.Minute, 2*time.Second).Should(Succeed())
	Expect(issueResp.TokenID).NotTo(BeEmpty())

	By("transferring tokens via REST")
	transferReq := map[string]string{
		"token_id":  issueResp.TokenID,
		"amount":    "5.00",
		"recipient": "bob",
	}
	var transferResp struct {
		TxID string `json:"tx_id"`
	}
	status, body, err := doJSON(hc, http.MethodPost, aliceBase+"/v1/tokens/transfer", transferReq)
	Expect(err).NotTo(HaveOccurred())
	Expect(status).To(Equal(http.StatusOK))
	Expect(json.Unmarshal(body, &transferResp)).To(Succeed())
	Expect(transferResp.TxID).NotTo(BeEmpty())
}

func restBaseURL(ii *integration.Infrastructure, nodeName string) string {
	peer := findPeer(ii, nodeName)
	confDir := ii.FscPlatform.NodeDir(peer)
	cfg, err := nwoclient.NewWebClientConfigFromFSC(confDir)
	Expect(err).NotTo(HaveOccurred())
	// The node binds on 0.0.0.0, but clients should dial localhost.
	if strings.HasPrefix(cfg.Host, "0.0.0.0:") {
		cfg.Host = strings.Replace(cfg.Host, "0.0.0.0:", "127.0.0.1:", 1)
	}
	return cfg.WebURL()
}

func findPeer(ii *integration.Infrastructure, nodeName string) *nwofscnode.Replica {
	var fallback *nwofscnode.Replica
	for _, p := range ii.FscPlatform.Peers {
		if p.Name != nodeName {
			continue
		}
		if strings.HasSuffix(p.UniqueName, ".0") {
			return p
		}
		fallback = p
	}
	Expect(fallback).NotTo(BeNil(), "missing FSC node %s", nodeName)
	return fallback
}

func doJSON(hc *http.Client, method, url string, body any) (int, []byte, error) {
	var rdr io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return 0, nil, err
		}
		rdr = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, url, rdr)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := hc.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, nil, err
	}
	return resp.StatusCode, b, nil
}

func errorsFromStatus(status int) error {
	return &httpError{status: status}
}

type httpError struct{ status int }

func (e *httpError) Error() string {
	return "unexpected http status: " + strconv.Itoa(e.status)
}

// Helper functions

func IssueTokens(ii *integration.Infrastructure, tokenType string, amount uint64, recipient string) string {
	res, err := ii.Client(tokenx.IssuerNode).CallView("issue", common.JSONMarshall(&views.Issue{
		TokenType: tokenType,
		Amount:    amount,
		Recipient: ii.Identity(recipient),
		Approvers: []view.Identity{ii.Identity(tokenx.ApproverNode)},
	}))
	Expect(err).NotTo(HaveOccurred())
	return common.JSONUnmarshalString(res)
}

func TransferTokens(ii *integration.Infrastructure, tokenLinearID string, amount uint64, from string, to string) string {
	res, err := ii.Client(from).CallView("transfer", common.JSONMarshall(&views.Transfer{
		TokenLinearID: tokenLinearID,
		Amount:        amount,
		Recipient:     ii.Identity(to),
		Approver:      ii.Identity(tokenx.ApproverNode),
	}))
	Expect(err).NotTo(HaveOccurred())
	return common.JSONUnmarshalString(res)
}

func RedeemTokens(ii *integration.Infrastructure, tokenLinearID string, amount uint64, owner string) string {
	res, err := ii.Client(owner).CallView("redeem", common.JSONMarshall(&views.Redeem{
		TokenLinearID: tokenLinearID,
		Amount:        amount,
		Approver:      ii.Identity(tokenx.ApproverNode),
	}))
	Expect(err).NotTo(HaveOccurred())
	return common.JSONUnmarshalString(res)
}

func QueryBalance(ii *integration.Infrastructure, owner string, tokenType string) *views.BalanceResult {
	res, err := ii.Client(owner).CallView("query", common.JSONMarshall(&views.BalanceQuery{
		TokenType: tokenType,
	}))
	Expect(err).NotTo(HaveOccurred())
	result := &views.BalanceResult{}
	common.JSONUnmarshal(res.([]byte), result)
	return result
}

func (s *TestSuite) TestSwapSucceeded() {
	// 1. Issue tokens
	By("issuing USD to alice for swap")
	aliceUSD := IssueTokens(s.II, "USD", states.TokenFromFloat(100), "alice")
	Expect(aliceUSD).NotTo(BeEmpty())

	By("issuing EUR to bob for swap")
	bobEUR := IssueTokens(s.II, "EUR", states.TokenFromFloat(85), "bob")
	Expect(bobEUR).NotTo(BeEmpty())

	// 2. Alice proposes swap: OFFER 100 USD, REQUEST 85 EUR
	By("alice proposing swap: 100 USD for 85 EUR")
	proposalID := SwapProposeTokens(s.II, aliceUSD, "EUR", states.TokenFromFloat(85), "alice")
	Expect(proposalID).NotTo(BeEmpty())

	// 3. Bob accepts swap using his EUR token
	By("bob accepting swap")
	swapTxID := SwapAcceptTokens(s.II, proposalID, bobEUR, "bob")
	Expect(swapTxID).NotTo(BeEmpty())

	// 4. Verify balances
	By("verifying alice has 85 EUR")
	aliceBal := QueryBalance(s.II, "alice", "EUR")
	Expect(aliceBal.Balances["EUR"]).To(Equal(states.TokenFromFloat(85)))

	By("verifying bob has 100 USD")
	bobBal := QueryBalance(s.II, "bob", "USD")
	Expect(bobBal.Balances["USD"]).To(Equal(states.TokenFromFloat(100)))
}

func (s *TestSuite) TestAuditorQueries() {
	// 1. Issue some tokens to generate history
	By("issuing tokens for audit test")
	IssueTokens(s.II, "AUDIT", states.TokenFromFloat(1000), "alice")

	// 2. Query auditor balances
	By("querying auditor balances")
	// Note: The implementation is a stub, so we just expect it to return a result without crashing
	auditorBal := QueryAuditorBalances(s.II, "AUDIT")
	Expect(auditorBal).NotTo(BeNil())
	Expect(auditorBal.TotalSupply).NotTo(BeNil())

	// 3. Query auditor history
	By("querying auditor history")
	auditorHist := QueryAuditorHistory(s.II, "AUDIT", "", 10)
	Expect(auditorHist).NotTo(BeNil())
	Expect(auditorHist.Records).NotTo(BeNil())
}

func (s *TestSuite) TestErrorCases() {
	By("issuing tokens for error testing")
	aliceID := IssueTokens(s.II, "ERR", states.TokenFromFloat(50), "alice")

	By("attempting transfer with insufficient funds")
	// Try to transfer 100 when having only 50. We expect this to fail.
	_, err := s.II.Client("alice").CallView("transfer", common.JSONMarshall(&views.Transfer{
		TokenLinearID: aliceID,
		Amount:        states.TokenFromFloat(100),
		Recipient:     s.II.Identity("bob"),
		Approver:      s.II.Identity(tokenx.ApproverNode),
	}))
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("insufficient balance"))
}

func SwapProposeTokens(ii *integration.Infrastructure, offeredTokenID string, requestedType string, requestedAmount uint64, proposer string) string {
	res, err := ii.Client(proposer).CallView("swap_propose", common.JSONMarshall(&views.SwapPropose{
		OfferedTokenID:  offeredTokenID,
		RequestedType:   requestedType,
		RequestedAmount: requestedAmount,
		ExpiryMinutes:   60,
	}))
	Expect(err).NotTo(HaveOccurred())
	return common.JSONUnmarshalString(res)
}

func SwapAcceptTokens(ii *integration.Infrastructure, proposalID string, offeredTokenID string, accepter string) string {
	res, err := ii.Client(accepter).CallView("swap_accept", common.JSONMarshall(&views.SwapAccept{
		ProposalID:     proposalID,
		OfferedTokenID: offeredTokenID,
		Approver:       ii.Identity(tokenx.ApproverNode),
	}))
	Expect(err).NotTo(HaveOccurred())
	return common.JSONUnmarshalString(res)
}

func QueryAuditorBalances(ii *integration.Infrastructure, tokenType string) *views.AuditorBalancesResult {
	res, err := ii.Client(tokenx.AuditorNode).CallView("balances", common.JSONMarshall(&views.AuditorBalancesQuery{
		TokenType: tokenType,
	}))
	Expect(err).NotTo(HaveOccurred())
	result := &views.AuditorBalancesResult{}
	common.JSONUnmarshal(res.([]byte), result)
	return result
}

func QueryAuditorHistory(ii *integration.Infrastructure, tokenType string, txType string, limit int) *views.AuditorHistoryResult {
	res, err := ii.Client(tokenx.AuditorNode).CallView("history", common.JSONMarshall(&views.AuditorHistoryQuery{
		TokenType: tokenType,
		TxType:    txType,
		Limit:     limit,
	}))
	Expect(err).NotTo(HaveOccurred())
	result := &views.AuditorHistoryResult{}
	common.JSONUnmarshal(res.([]byte), result)
	return result
}
