/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package api

import (
	"net/http"

	"github.com/hyperledger-labs/fabric-smart-client/examples/tickets/views"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
)

// statusOf turns what went wrong in a view into the HTTP status and error code
// an existing system understands. Both 409s mean "this seat is gone", but the
// code says which layer stopped the sale: the check before the promoter
// endorsed (already_sold), or the ledger at commit (lost_race).
func statusOf(err error) (status int, code string) {
	for _, m := range []struct {
		err    error
		status int
		code   string
	}{
		{views.ErrInvalidInput, http.StatusBadRequest, "bad_request"},
		{views.ErrUnknownEvent, http.StatusNotFound, "not_found"},
		{views.ErrSeatSold, http.StatusConflict, "already_sold"},
		{views.ErrLostRace, http.StatusConflict, "lost_race"},
		{views.ErrConflict, http.StatusConflict, "conflict"},
		{views.ErrRefused, http.StatusUnprocessableEntity, "refused"},
		{views.ErrUnreachable, http.StatusServiceUnavailable, "unavailable"},
		{views.ErrPending, http.StatusGatewayTimeout, "pending"},
	} {
		if errors.Is(err, m.err) {
			return m.status, m.code
		}
	}
	return http.StatusInternalServerError, "internal"
}
