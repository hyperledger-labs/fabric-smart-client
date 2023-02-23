/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package optl

const ConfigTemplate = `receivers:
  otlp:
    protocols:
      http:
        endpoint: 0.0.0.0:4319    
      # grpc:
      #   endpoint: 0.0.0.0:4320

exporters:
  # prometheus:
  #   endpoint: "0.0.0.0:8889"
  #   const_labels:
  #     label1: value1

  logging:
    loglevel: debug
    
  # zipkin:
  #   endpoint: "http://zipkin-all-in-one:9411/api/v2/spans"
  #   format: proto

  jaeger:
    endpoint: jaegertracing.mynetwork.com:14250
    tls:
      insecure: true

  file:
    path: "./filename.json"

processors:
  batch:

extensions:
  health_check:
  pprof:
    endpoint: :1888
  zpages:
    endpoint: :55679

service:
  extensions: [pprof, zpages, health_check]
  pipelines:
    traces:
      receivers: [otlp]
      processors: []
      exporters: [logging,jaeger]
    # metrics:
    #   receivers: [otlp]
    #   processors: [batch]
    #   exporters: [logging, prometheus]
`
