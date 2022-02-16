/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package monitoring

const DatasourceTemplate = `apiVersion: 1

datasources:
  - name: 'Prometheus'
    type: 'prometheus'
    access: 'proxy'
    orgId: 1
    url: 'http://prometheus:9090'
    httpMethod: GET
    isDefault: true
    version: 1
    editable: true
`
const DashboardTemplate = `apiVersion: 1

providers:
- name: 'default'
  orgId: 1
  folder: ''
  type: file
  disableDeletion: false
  updateIntervalSeconds: 10 #how often Grafana will scan for changed dashboards
  options:
    path: /var/lib/grafana/dashboards`
