/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package p2p

import "time"

// SanitizeDetail exposes sanitizeDetail to the external test package.
var SanitizeDetail = sanitizeDetail

// WithAuthorizeTimeout sets the bound on a responder authorizer's call.
func WithAuthorizeTimeout(d time.Duration) Option {
	return func(s *Service) { s.authorizeTimeout = d }
}
