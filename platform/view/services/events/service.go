/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package events

// Service exposes an [EventSystem] as a separate [Publisher] and [Subscriber].
type Service struct {
	EventSystem EventSystem
}

// GetSubscriber returns the underlying [EventSystem] as a [Subscriber].
func (s *Service) GetSubscriber() Subscriber {
	return s.EventSystem
}

// GetPublisher returns the underlying [EventSystem] as a [Publisher].
func (s *Service) GetPublisher() Publisher {
	return s.EventSystem
}
