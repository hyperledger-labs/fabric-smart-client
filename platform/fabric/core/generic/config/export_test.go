/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package config

import "time"

const EndpointCooldown = endpointCooldown

func (s *Service) SetNow(now func() time.Time) { s.now = now }
