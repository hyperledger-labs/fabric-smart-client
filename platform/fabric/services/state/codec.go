/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package state

type Marshaller interface {
	Marshal(v interface{}) ([]byte, error)
}

type Unmarshaller interface {
	Unmarshal(data []byte, v interface{}) error
}

type Codec interface {
	Marshaller
	Unmarshaller
}

func Unmarshal(unmarshaller Unmarshaller, data []byte, v interface{}) error {
	return unmarshaller.Unmarshal(data, v)
}
