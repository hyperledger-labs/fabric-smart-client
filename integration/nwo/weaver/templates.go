/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package weaver

const RelayServerTOML = `# Name of the relay
name = "{{ Name }}"
# Port number for relay grpc server. e.g. 9080
port="{{ Port }}"
# Host address for grpc server. e.g. 0.0.0.0
hostname="{{ Hostname }}"

db_path="db/{{ Name }}/requests"
# This will be replaced by the task queue.
remote_db_path="db/{{ Name }}/remote_request"

# FOR TLS
cert_path="credentials/fabric_cert.pem"
key_path="credentials/fabric_key"
# tls=true

[networks]{{ range Networks }}
[networks.{{ .Name }}]
network="{{ .Type }}"
{{- end }}

[relays]{{ range Relays }}
[relays.{{ .Name }}]
hostname="{{ .Hostname }}"
port="{{ .Port }}"
{{- end }}

[drivers]{{ range Drivers }}
[drivers.{{ .Name }}]
hostname="{{ .Hostname }}"
port="{{ .Port }}"
{{- end }}
`

const RelayFabricDriverConnectionConfig = ``

const RelayFabricDriverConfig = `{
    "admin":{
        "name":"admin",
        "secret":"adminpw"
    },
    "relay": {
        "name":"relay",
        "affiliation":"org1.department1",
        "role": "client",
        "attrs": [{ "name": "relay", "value": "true", "ecert": true }]
    },
    "mspId":"Org1MSP",
    "caUrl":""
}
`
