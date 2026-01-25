/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/hyperledger-labs/fabric-smart-client/integration/fabricx/tokenx/states"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabricx/tokenx/views"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	server "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/web/server"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

var logger = logging.MustGetLogger()

// TokenAPI provides REST API handlers for token operations
type TokenAPI struct {
	viewCaller      ViewCaller
	endpointService EndpointService
}

// ViewCaller is an interface for calling FSC views
type ViewCaller interface {
	CallView(ctx context.Context, vid string, input []byte) (interface{}, error)
}

// EndpointService resolves endpoint labels (e.g., node names) to FSC identities.
type EndpointService interface {
	GetIdentity(label string, pkID []byte) (view.Identity, error)
}

// NewTokenAPI creates a new TokenAPI instance
func NewTokenAPI(viewCaller ViewCaller, endpointService EndpointService) *TokenAPI {
	return &TokenAPI{viewCaller: viewCaller, endpointService: endpointService}
}

// RegisterHandlers registers all API handlers with the HTTP handler
func (api *TokenAPI) RegisterHandlers(h *server.HttpHandler) {
	// Token operations
	h.RegisterURI("/tokens/issue", http.MethodPost, api.IssueHandler())
	h.RegisterURI("/tokens/transfer", http.MethodPost, api.TransferHandler())
	h.RegisterURI("/tokens/redeem", http.MethodPost, api.RedeemHandler())
	h.RegisterURI("/tokens/balance", http.MethodGet, api.BalanceHandler())
	h.RegisterURI("/tokens/history", http.MethodGet, api.HistoryHandler())

	// Swap operations
	h.RegisterURI("/tokens/swap/propose", http.MethodPost, api.SwapProposeHandler())
	h.RegisterURI("/tokens/swap/accept", http.MethodPost, api.SwapAcceptHandler())

	// Audit operations
	h.RegisterURI("/audit/balances", http.MethodGet, api.AuditBalancesHandler())
	h.RegisterURI("/audit/history", http.MethodGet, api.AuditHistoryHandler())

	logger.Infof("TokenAPI handlers registered")
}

// IssueRequest is the request body for issuing tokens
type IssueRequest struct {
	TokenType string `json:"token_type"`
	Amount    string `json:"amount"` // String to support decimal input
	Recipient string `json:"recipient"`
}

// IssueHandler handles POST /tokens/issue
func (api *TokenAPI) IssueHandler() server.RequestHandler {
	return &issueHandler{api: api}
}

type issueHandler struct {
	api *TokenAPI
}

func (h *issueHandler) ParsePayload(data []byte) (interface{}, error) {
	var req IssueRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, err
	}
	return &req, nil
}

func (h *issueHandler) HandleRequest(ctx *server.ReqContext) (interface{}, int) {
	req := ctx.Query.(*IssueRequest)
	if len(req.Recipient) == 0 {
		return map[string]string{"error": "missing recipient"}, http.StatusBadRequest
	}

	// Convert amount string to uint64
	amountFloat, err := strconv.ParseFloat(req.Amount, 64)
	if err != nil {
		return map[string]string{"error": "invalid amount: " + err.Error()}, http.StatusBadRequest
	}
	amount := states.TokenFromFloat(amountFloat)

	recipient, err := h.api.endpointService.GetIdentity(req.Recipient, nil)
	if err != nil {
		return map[string]string{"error": "unknown recipient: " + err.Error()}, http.StatusBadRequest
	}
	approver, err := h.api.endpointService.GetIdentity("approver", nil)
	if err != nil {
		return map[string]string{"error": "failed resolving approver: " + err.Error()}, http.StatusBadRequest
	}

	input, _ := json.Marshal(&views.Issue{
		TokenType: req.TokenType,
		Amount:    amount,
		Recipient: recipient,
		Approvers: []view.Identity{approver},
	})

	result, err := h.api.viewCaller.CallView(ctx.Req.Context(), "issue", input)
	if err != nil {
		return map[string]string{"error": err.Error()}, http.StatusBadRequest
	}

	return map[string]interface{}{"token_id": result}, http.StatusOK
}

// TransferRequest is the request body for transferring tokens
type TransferRequest struct {
	TokenID   string `json:"token_id"`
	Amount    string `json:"amount"`
	Recipient string `json:"recipient"`
}

// TransferHandler handles POST /tokens/transfer
func (api *TokenAPI) TransferHandler() server.RequestHandler {
	return &transferHandler{api: api}
}

type transferHandler struct {
	api *TokenAPI
}

func (h *transferHandler) ParsePayload(data []byte) (interface{}, error) {
	var req TransferRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, err
	}
	return &req, nil
}

func (h *transferHandler) HandleRequest(ctx *server.ReqContext) (interface{}, int) {
	req := ctx.Query.(*TransferRequest)
	if len(req.TokenID) == 0 {
		return map[string]string{"error": "missing token_id"}, http.StatusBadRequest
	}
	if len(req.Recipient) == 0 {
		return map[string]string{"error": "missing recipient"}, http.StatusBadRequest
	}

	// Convert amount string to uint64
	amountFloat, err := strconv.ParseFloat(req.Amount, 64)
	if err != nil {
		return map[string]string{"error": "invalid amount: " + err.Error()}, http.StatusBadRequest
	}
	amount := states.TokenFromFloat(amountFloat)

	recipient, err := h.api.endpointService.GetIdentity(req.Recipient, nil)
	if err != nil {
		return map[string]string{"error": "unknown recipient: " + err.Error()}, http.StatusBadRequest
	}
	approver, err := h.api.endpointService.GetIdentity("approver", nil)
	if err != nil {
		return map[string]string{"error": "failed resolving approver: " + err.Error()}, http.StatusBadRequest
	}

	input, _ := json.Marshal(&views.Transfer{
		TokenLinearID: req.TokenID,
		Amount:        amount,
		Recipient:     recipient,
		Approver:      approver,
	})

	result, err := h.api.viewCaller.CallView(ctx.Req.Context(), "transfer", input)
	if err != nil {
		return map[string]string{"error": err.Error()}, http.StatusBadRequest
	}

	return map[string]interface{}{"tx_id": result}, http.StatusOK
}

// RedeemRequest is the request body for redeeming tokens
type RedeemRequest struct {
	TokenID string `json:"token_id"`
	Amount  string `json:"amount"`
}

// RedeemHandler handles POST /tokens/redeem
func (api *TokenAPI) RedeemHandler() server.RequestHandler {
	return &redeemHandler{api: api}
}

type redeemHandler struct {
	api *TokenAPI
}

func (h *redeemHandler) ParsePayload(data []byte) (interface{}, error) {
	var req RedeemRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, err
	}
	return &req, nil
}

func (h *redeemHandler) HandleRequest(ctx *server.ReqContext) (interface{}, int) {
	req := ctx.Query.(*RedeemRequest)
	if len(req.TokenID) == 0 {
		return map[string]string{"error": "missing token_id"}, http.StatusBadRequest
	}

	// Convert amount string to uint64
	amountFloat, err := strconv.ParseFloat(req.Amount, 64)
	if err != nil {
		return map[string]string{"error": "invalid amount: " + err.Error()}, http.StatusBadRequest
	}
	amount := states.TokenFromFloat(amountFloat)

	approver, err := h.api.endpointService.GetIdentity("approver", nil)
	if err != nil {
		return map[string]string{"error": "failed resolving approver: " + err.Error()}, http.StatusBadRequest
	}

	input, _ := json.Marshal(&views.Redeem{
		TokenLinearID: req.TokenID,
		Amount:        amount,
		Approver:      approver,
	})

	result, err := h.api.viewCaller.CallView(ctx.Req.Context(), "redeem", input)
	if err != nil {
		return map[string]string{"error": err.Error()}, http.StatusBadRequest
	}

	return map[string]interface{}{"tx_id": result}, http.StatusOK
}

// BalanceHandler handles GET /tokens/balance
func (api *TokenAPI) BalanceHandler() server.RequestHandler {
	return &balanceHandler{api: api}
}

type balanceHandler struct {
	api *TokenAPI
}

func (h *balanceHandler) ParsePayload(data []byte) (interface{}, error) {
	return nil, nil
}

func (h *balanceHandler) HandleRequest(ctx *server.ReqContext) (interface{}, int) {
	tokenType := ctx.Req.URL.Query().Get("token_type")

	input, _ := json.Marshal(&views.BalanceQuery{
		TokenType: tokenType,
	})

	result, err := h.api.viewCaller.CallView(ctx.Req.Context(), "query", input)
	if err != nil {
		return map[string]string{"error": err.Error()}, http.StatusBadRequest
	}

	return result, http.StatusOK
}

// HistoryHandler handles GET /tokens/history
func (api *TokenAPI) HistoryHandler() server.RequestHandler {
	return &historyHandler{api: api}
}

type historyHandler struct {
	api *TokenAPI
}

func (h *historyHandler) ParsePayload(data []byte) (interface{}, error) {
	return nil, nil
}

func (h *historyHandler) HandleRequest(ctx *server.ReqContext) (interface{}, int) {
	tokenType := ctx.Req.URL.Query().Get("token_type")
	txType := ctx.Req.URL.Query().Get("tx_type")

	input, _ := json.Marshal(&views.OwnerHistoryQuery{
		TokenType: tokenType,
		TxType:    txType,
	})

	result, err := h.api.viewCaller.CallView(ctx.Req.Context(), "history", input)
	if err != nil {
		return map[string]string{"error": err.Error()}, http.StatusBadRequest
	}

	return result, http.StatusOK
}

// SwapProposeHandler handles POST /tokens/swap/propose
func (api *TokenAPI) SwapProposeHandler() server.RequestHandler {
	return &swapProposeHandler{api: api}
}

type swapProposeHandler struct {
	api *TokenAPI
}

func (h *swapProposeHandler) ParsePayload(data []byte) (interface{}, error) {
	var req views.SwapPropose
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, err
	}
	return &req, nil
}

func (h *swapProposeHandler) HandleRequest(ctx *server.ReqContext) (interface{}, int) {
	req := ctx.Query.(*views.SwapPropose)

	input, _ := json.Marshal(req)

	result, err := h.api.viewCaller.CallView(ctx.Req.Context(), "swap_propose", input)
	if err != nil {
		return map[string]string{"error": err.Error()}, http.StatusBadRequest
	}

	return map[string]interface{}{"proposal_id": result}, http.StatusOK
}

// SwapAcceptHandler handles POST /tokens/swap/accept
func (api *TokenAPI) SwapAcceptHandler() server.RequestHandler {
	return &swapAcceptHandler{api: api}
}

type swapAcceptHandler struct {
	api *TokenAPI
}

func (h *swapAcceptHandler) ParsePayload(data []byte) (interface{}, error) {
	var req views.SwapAccept
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, err
	}
	return &req, nil
}

func (h *swapAcceptHandler) HandleRequest(ctx *server.ReqContext) (interface{}, int) {
	req := ctx.Query.(*views.SwapAccept)

	approver, err := h.api.endpointService.GetIdentity("approver", nil)
	if err != nil {
		return map[string]string{"error": "failed resolving approver: " + err.Error()}, http.StatusBadRequest
	}
	req.Approver = approver

	input, _ := json.Marshal(req)

	result, err := h.api.viewCaller.CallView(ctx.Req.Context(), "swap_accept", input)
	if err != nil {
		return map[string]string{"error": err.Error()}, http.StatusBadRequest
	}

	return map[string]interface{}{"tx_id": result}, http.StatusOK
}

// AuditBalancesHandler handles GET /audit/balances
func (api *TokenAPI) AuditBalancesHandler() server.RequestHandler {
	return &auditBalancesHandler{api: api}
}

type auditBalancesHandler struct {
	api *TokenAPI
}

func (h *auditBalancesHandler) ParsePayload(data []byte) (interface{}, error) {
	return nil, nil
}

func (h *auditBalancesHandler) HandleRequest(ctx *server.ReqContext) (interface{}, int) {
	tokenType := ctx.Req.URL.Query().Get("token_type")

	input, _ := json.Marshal(&views.AuditorBalancesQuery{
		TokenType: tokenType,
	})

	result, err := h.api.viewCaller.CallView(ctx.Req.Context(), "balances", input)
	if err != nil {
		return map[string]string{"error": err.Error()}, http.StatusBadRequest
	}

	return result, http.StatusOK
}

// AuditHistoryHandler handles GET /audit/history
func (api *TokenAPI) AuditHistoryHandler() server.RequestHandler {
	return &auditHistoryHandler{api: api}
}

type auditHistoryHandler struct {
	api *TokenAPI
}

func (h *auditHistoryHandler) ParsePayload(data []byte) (interface{}, error) {
	return nil, nil
}

func (h *auditHistoryHandler) HandleRequest(ctx *server.ReqContext) (interface{}, int) {
	tokenType := ctx.Req.URL.Query().Get("token_type")
	txType := ctx.Req.URL.Query().Get("tx_type")

	input, _ := json.Marshal(&views.AuditorHistoryQuery{
		TokenType: tokenType,
		TxType:    txType,
	})

	result, err := h.api.viewCaller.CallView(ctx.Req.Context(), "history", input)
	if err != nil {
		return map[string]string{"error": err.Error()}, http.StatusBadRequest
	}

	return result, http.StatusOK
}
