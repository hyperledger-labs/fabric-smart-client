/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabric

import "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"

type ConfigService struct {
	confService driver.ConfigService
}

func (s *ConfigService) DriverName() string {
	return s.confService.DriverName()
}

func (s *ConfigService) GetString(key string) string {
	return s.confService.GetString(key)
}

func (s *ConfigService) DefaultChannel() string {
	return s.confService.DefaultChannel()
}
