/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package web

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
	"github.com/pkg/errors"
)

var webLogger = flogging.MustGetLogger("view-sdk.server.web")

func New(sp view.ServiceProvider) (*Server, error) {
	configProvider := view.GetConfigService(sp)

	listenAddr := configProvider.GetString("fsc.web.address")

	var tlsConfig TLS
	prefix := "fsc."
	if configProvider.IsSet("fsc.web.tls") {
		prefix = "fsc.web."
	}
	var clientRootCAs []string
	for _, path := range configProvider.GetStringSlice(prefix + "tls.clientRootCAs.files") {
		clientRootCAs = append(clientRootCAs, configProvider.TranslatePath(path))
	}
	tlsConfig = TLS{
		Enabled:           configProvider.GetBool(prefix + "tls.enabled"),
		CertFile:          configProvider.GetPath(prefix + "tls.cert.file"),
		KeyFile:           configProvider.GetPath(prefix + "tls.key.file"),
		ClientCACertFiles: clientRootCAs,
	}
	webServer := NewServer(Options{
		ListenAddress: listenAddr,
		Logger:        webLogger,
		TLS:           tlsConfig,
	})
	h := NewHttpHandler(webLogger)
	webServer.RegisterHandler("/", h, true)

	d := &Dispatcher{
		Logger:  webLogger,
		Handler: h,
	}
	InstallViewHandler(webLogger, sp, d)

	return webServer, nil
}

var webServiceLookUp = &Server{}

func GetService(sp view.ServiceProvider) (*Server, error) {
	s, err := sp.GetService(webServiceLookUp)
	if err != nil {
		return nil, errors.Wrap(err, "cannot get web Service from registry")
	}
	return s.(*Server), nil
}
