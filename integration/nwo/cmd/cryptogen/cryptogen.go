/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package cryptogen

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"text/template"

	ca2 "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/cmd/cryptogen/ca"
	msp2 "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/cmd/cryptogen/msp"
	"gopkg.in/yaml.v2"
)

const (
	userBaseName            = "User"
	adminBaseName           = "Admin"
	defaultHostnameTemplate = "{{.Prefix}}{{.Index}}"
	defaultCNTemplate       = "{{.Hostname}}.{{.Domain}}"
)

type HostnameData struct {
	Prefix string
	Index  int
	Domain string
}

type SpecData struct {
	Hostname   string
	Domain     string
	CommonName string
}

type NodeTemplate struct {
	Count    int      `yaml:"Count"`
	Start    int      `yaml:"Start"`
	Hostname string   `yaml:"Hostname"`
	SANS     []string `yaml:"SANS"`
}

type NodeSpec struct {
	isAdmin            bool
	Hostname           string   `yaml:"Hostname"`
	CommonName         string   `yaml:"CommonName"`
	Country            string   `yaml:"Country"`
	Province           string   `yaml:"Province"`
	Locality           string   `yaml:"Locality"`
	OrganizationalUnit string   `yaml:"OrganizationalUnit"`
	StreetAddress      string   `yaml:"StreetAddress"`
	PostalCode         string   `yaml:"PostalCode"`
	SANS               []string `yaml:"SANS"`
	HSM                bool     `yaml:"HSM"`
}

type UserSpec struct {
	Name string `yaml:"Name"`
	HSM  bool   `yaml:"HSM"`
}

type UsersSpec struct {
	Count int        `yaml:"Count"`
	Specs []UserSpec `yaml:"Specs"`
}

type OrgSpec struct {
	Name          string       `yaml:"Name"`
	Domain        string       `yaml:"Domain"`
	EnableNodeOUs bool         `yaml:"EnableNodeOUs"`
	CA            NodeSpec     `yaml:"CA"`
	Template      NodeTemplate `yaml:"Template"`
	Specs         []NodeSpec   `yaml:"Specs"`
	Users         UsersSpec    `yaml:"Users"`
}

type Config struct {
	OrdererOrgs []OrgSpec `yaml:"OrdererOrgs"`
	PeerOrgs    []OrgSpec `yaml:"PeerOrgs"`
}

var defaultConfig = `
# ---------------------------------------------------------------------------
# "OrdererOrgs" - Definition of organizations managing orderer nodes
# ---------------------------------------------------------------------------
OrdererOrgs:
  # ---------------------------------------------------------------------------
  # Orderer
  # ---------------------------------------------------------------------------
  - Name: Orderer
    Domain: example.com
    EnableNodeOUs: false

    # ---------------------------------------------------------------------------
    # "Specs" - See PeerOrgs below for complete description
    # ---------------------------------------------------------------------------
    Specs:
      - Hostname: orderer

# ---------------------------------------------------------------------------
# "PeerOrgs" - Definition of organizations managing peer nodes
# ---------------------------------------------------------------------------
PeerOrgs:
  # ---------------------------------------------------------------------------
  # Org1
  # ---------------------------------------------------------------------------
  - Name: Org1
    Domain: org1.example.com
    EnableNodeOUs: false

    # ---------------------------------------------------------------------------
    # "CA"
    # ---------------------------------------------------------------------------
    # Uncomment this section to enable the explicit definition of the CA for this
    # organization.  This entry is a Spec.  See "Specs" section below for details.
    # ---------------------------------------------------------------------------
    # CA:
    #    Hostname: ca # implicitly ca.org1.example.com
    #    Country: US
    #    Province: California
    #    Locality: San Francisco
    #    OrganizationalUnit: Hyperledger Fabric
    #    StreetAddress: address for org # default nil
    #    PostalCode: postalCode for org # default nil

    # ---------------------------------------------------------------------------
    # "Specs"
    # ---------------------------------------------------------------------------
    # Uncomment this section to enable the explicit definition of hosts in your
    # configuration.  Most users will want to use Template, below
    #
    # Specs is an array of Spec entries.  Each Spec entry consists of two fields:
    #   - Hostname:   (Required) The desired hostname, sans the domain.
    #   - CommonName: (Optional) Specifies the template or explicit override for
    #                 the CN.  By default, this is the template:
    #
    #                              "{{.Hostname}}.{{.Domain}}"
    #
    #                 which obtains its values from the Spec.Hostname and
    #                 Org.Domain, respectively.
    #   - SANS:       (Optional) Specifies one or more Subject Alternative Names
    #                 to be set in the resulting x509. Accepts template
    #                 variables {{.Hostname}}, {{.Domain}}, {{.CommonName}}. IP
    #                 addresses provided here will be properly recognized. Other
    #                 values will be taken as DNS names.
    #                 NOTE: Two implicit entries are created for you:
    #                     - {{ .CommonName }}
    #                     - {{ .Hostname }}
    # ---------------------------------------------------------------------------
    # Specs:
    #   - Hostname: foo # implicitly "foo.org1.example.com"
    #     CommonName: foo27.org5.example.com # overrides Hostname-based FQDN set above
    #     SANS:
    #       - "bar.{{.Domain}}"
    #       - "altfoo.{{.Domain}}"
    #       - "{{.Hostname}}.org6.net"
    #       - 172.16.10.31
    #   - Hostname: bar
    #   - Hostname: baz

    # ---------------------------------------------------------------------------
    # "Template"
    # ---------------------------------------------------------------------------
    # Allows for the definition of 1 or more hosts that are created sequentially
    # from a template. By default, this looks like "peer%d" from 0 to Count-1.
    # You may override the number of nodes (Count), the starting index (Start)
    # or the template used to construct the name (Hostname).
    #
    # Note: Template and Specs are not mutually exclusive.  You may define both
    # sections and the aggregate nodes will be created for you.  Take care with
    # name collisions
    # ---------------------------------------------------------------------------
    Template:
      Count: 1
      # Start: 5
      # Hostname: {{.Prefix}}{{.Index}} # default
      # SANS:
      #   - "{{.Hostname}}.alt.{{.Domain}}"

    # ---------------------------------------------------------------------------
    # "Users"
    # ---------------------------------------------------------------------------
    # Count: The number of user accounts _in addition_ to Admin
    # ---------------------------------------------------------------------------
    Users:
      Count: 1

  # ---------------------------------------------------------------------------
  # Org2: See "Org1" for full specification
  # ---------------------------------------------------------------------------
  - Name: Org2
    Domain: org2.example.com
    EnableNodeOUs: false
    Template:
      Count: 1
    Users:
      Count: 1
`

func getConfig() (*Config, error) {
	var configData string

	if genConfigFile != "" {
		data, err := os.ReadFile(genConfigFile)
		if err != nil {
			return nil, fmt.Errorf("error reading configuration: %s", err)
		}

		configData = string(data)
	} else if extConfigFile != "" {
		data, err := os.ReadFile(extConfigFile)
		if err != nil {
			return nil, fmt.Errorf("error reading configuration: %s", err)
		}

		configData = string(data)
	} else {
		configData = defaultConfig
	}

	config := &Config{}
	err := yaml.Unmarshal([]byte(configData), &config)
	if err != nil {
		return nil, fmt.Errorf("error Unmarshaling YAML: %s", err)
	}

	return config, nil
}

func generate() {
	config, err := getConfig()
	if err != nil {
		fmt.Printf("Error reading config: %s", err)
		os.Exit(-1)
	}

	for _, orgSpec := range config.PeerOrgs {
		err = renderOrgSpec(&orgSpec, "peer")
		if err != nil {
			fmt.Printf("Error processing peer configuration: %s", err)
			os.Exit(-1)
		}
		generatePeerOrg(outputDir, orgSpec)
	}

	for _, orgSpec := range config.OrdererOrgs {
		err = renderOrgSpec(&orgSpec, "orderer")
		if err != nil {
			fmt.Printf("Error processing orderer configuration: %s", err)
			os.Exit(-1)
		}
		generateOrdererOrg(outputDir, orgSpec)
	}
}

func parseTemplate(input string, data interface{}) (string, error) {

	t, err := template.New("parse").Parse(input)
	if err != nil {
		return "", fmt.Errorf("error parsing template: %s", err)
	}

	output := new(bytes.Buffer)
	err = t.Execute(output, data)
	if err != nil {
		return "", fmt.Errorf("error executing template: %s", err)
	}

	return output.String(), nil
}

func parseTemplateWithDefault(input, defaultInput string, data interface{}) (string, error) {

	// Use the default if the input is an empty string
	if len(input) == 0 {
		input = defaultInput
	}

	return parseTemplate(input, data)
}

func renderNodeSpec(domain string, spec *NodeSpec) error {
	data := SpecData{
		Hostname: spec.Hostname,
		Domain:   domain,
	}

	// Process our CommonName
	cn, err := parseTemplateWithDefault(spec.CommonName, defaultCNTemplate, data)
	if err != nil {
		return err
	}

	spec.CommonName = cn
	data.CommonName = cn

	// Save off our original, unprocessed SANS entries
	origSANS := spec.SANS

	// Set our implicit SANS entries for CN/Hostname
	spec.SANS = []string{cn, spec.Hostname}

	// Finally, process any remaining SANS entries
	for _, _san := range origSANS {
		san, err := parseTemplate(_san, data)
		if err != nil {
			return err
		}

		spec.SANS = append(spec.SANS, san)
	}

	return nil
}

func renderOrgSpec(orgSpec *OrgSpec, prefix string) error {
	// First process all of our templated nodes
	for i := 0; i < orgSpec.Template.Count; i++ {
		data := HostnameData{
			Prefix: prefix,
			Index:  i + orgSpec.Template.Start,
			Domain: orgSpec.Domain,
		}

		hostname, err := parseTemplateWithDefault(orgSpec.Template.Hostname, defaultHostnameTemplate, data)
		if err != nil {
			return err
		}

		spec := NodeSpec{
			Hostname: hostname,
			SANS:     orgSpec.Template.SANS,
		}
		orgSpec.Specs = append(orgSpec.Specs, spec)
	}

	// Touch up all general node-specs to add the domain
	for idx, spec := range orgSpec.Specs {
		err := renderNodeSpec(orgSpec.Domain, &spec)
		if err != nil {
			return err
		}

		orgSpec.Specs[idx] = spec
	}

	// Process the CA node-spec in the same manner
	if len(orgSpec.CA.Hostname) == 0 {
		orgSpec.CA.Hostname = "ca"
	}
	err := renderNodeSpec(orgSpec.Domain, &orgSpec.CA)
	if err != nil {
		return err
	}

	return nil
}

func generatePeerOrg(baseDir string, orgSpec OrgSpec) {

	orgName := orgSpec.Domain

	fmt.Println(orgName)
	// generate CAs
	orgDir := filepath.Join(baseDir, "peerOrganizations", orgName)
	caDir := filepath.Join(orgDir, "ca")
	tlsCADir := filepath.Join(orgDir, "tlsca")
	mspDir := filepath.Join(orgDir, "msp")
	peersDir := filepath.Join(orgDir, "peers")
	usersDir := filepath.Join(orgDir, "users")
	adminCertsDir := filepath.Join(mspDir, "admincerts")
	// generate signing CA
	signCA, err := ca2.NewCA(caDir, orgName, orgSpec.CA.CommonName, orgSpec.CA.Country, orgSpec.CA.Province, orgSpec.CA.Locality, orgSpec.CA.OrganizationalUnit, orgSpec.CA.StreetAddress, orgSpec.CA.PostalCode)
	if err != nil {
		fmt.Printf("Error generating signCA for org %s:\n%v\n", orgName, err)
		os.Exit(1)
	}
	// generate TLS CA
	tlsCA, err := ca2.NewCA(tlsCADir, orgName, "tls"+orgSpec.CA.CommonName, orgSpec.CA.Country, orgSpec.CA.Province, orgSpec.CA.Locality, orgSpec.CA.OrganizationalUnit, orgSpec.CA.StreetAddress, orgSpec.CA.PostalCode)
	if err != nil {
		fmt.Printf("Error generating tlsCA for org %s:\n%v\n", orgName, err)
		os.Exit(1)
	}

	err = msp2.GenerateVerifyingMSP(mspDir, signCA, tlsCA, orgSpec.EnableNodeOUs)
	if err != nil {
		fmt.Printf("Error generating MSP for org %s:\n%v\n", orgName, err)
		os.Exit(1)
	}

	generateNodes(peersDir, orgSpec.Specs, signCA, tlsCA, msp2.PEER, orgSpec.EnableNodeOUs)

	var users []NodeSpec
	if len(orgSpec.Users.Specs) != 0 {
		for j := 0; j < len(orgSpec.Users.Specs); j++ {
			user := NodeSpec{
				CommonName: fmt.Sprintf("%s@%s", orgSpec.Users.Specs[j].Name, orgName),
				HSM:        orgSpec.Users.Specs[j].HSM,
			}
			users = append(users, user)
		}
	} else {
		for j := 1; j <= orgSpec.Users.Count; j++ {
			user := NodeSpec{
				CommonName: fmt.Sprintf("%s%d@%s", userBaseName, j, orgName),
			}
			users = append(users, user)
		}
	}

	// add an admin user
	adminUser := NodeSpec{
		isAdmin:    true,
		CommonName: fmt.Sprintf("%s@%s", adminBaseName, orgName),
	}

	users = append(users, adminUser)
	generateNodes(usersDir, users, signCA, tlsCA, msp2.CLIENT, orgSpec.EnableNodeOUs)

	// copy the admin cert to the org's MSP admincerts
	if !orgSpec.EnableNodeOUs {
		err = copyAdminCert(usersDir, adminCertsDir, adminUser.CommonName)
		if err != nil {
			fmt.Printf("Error copying admin cert for org %s:\n%v\n",
				orgName, err)
			os.Exit(1)
		}
	}

	// copy the admin cert to each of the org's peer's MSP admincerts
	for _, spec := range orgSpec.Specs {
		if orgSpec.EnableNodeOUs {
			continue
		}
		err = copyAdminCert(usersDir,
			filepath.Join(peersDir, spec.CommonName, "msp", "admincerts"), adminUser.CommonName)
		if err != nil {
			fmt.Printf("Error copying admin cert for org %s peer %s:\n%v\n",
				orgName, spec.CommonName, err)
			os.Exit(1)
		}
	}
}

func copyAdminCert(usersDir, adminCertsDir, adminUserName string) error {
	if _, err := os.Stat(filepath.Join(adminCertsDir,
		adminUserName+"-cert.pem")); err == nil {
		return nil
	}
	// delete the contents of admincerts
	err := os.RemoveAll(adminCertsDir)
	if err != nil {
		return err
	}
	// recreate the admincerts directory
	err = os.MkdirAll(adminCertsDir, 0755)
	if err != nil {
		return err
	}
	err = copyFile(filepath.Join(usersDir, adminUserName, "msp", "signcerts",
		adminUserName+"-cert.pem"), filepath.Join(adminCertsDir,
		adminUserName+"-cert.pem"))
	if err != nil {
		return err
	}
	return nil
}

func generateNodes(baseDir string, nodes []NodeSpec, signCA *ca2.CA, tlsCA *ca2.CA, nodeType int, nodeOUs bool) {
	for _, node := range nodes {
		nodeDir := filepath.Join(baseDir, node.CommonName)
		if _, err := os.Stat(nodeDir); os.IsNotExist(err) {
			currentNodeType := nodeType
			if node.isAdmin && nodeOUs {
				currentNodeType = msp2.ADMIN
			}
			err := msp2.GenerateLocalMSP(nodeDir, node.CommonName, node.SANS, signCA, tlsCA, currentNodeType, nodeOUs, node.HSM, nil)
			if err != nil {
				fmt.Printf("Error generating local MSP for %v:\n%v\n", node, err)
				os.Exit(1)
			}
		}
	}
}

func generateOrdererOrg(baseDir string, orgSpec OrgSpec) {

	orgName := orgSpec.Domain

	// generate CAs
	orgDir := filepath.Join(baseDir, "ordererOrganizations", orgName)
	caDir := filepath.Join(orgDir, "ca")
	tlsCADir := filepath.Join(orgDir, "tlsca")
	mspDir := filepath.Join(orgDir, "msp")
	orderersDir := filepath.Join(orgDir, "orderers")
	usersDir := filepath.Join(orgDir, "users")
	adminCertsDir := filepath.Join(mspDir, "admincerts")
	// generate signing CA
	signCA, err := ca2.NewCA(caDir, orgName, orgSpec.CA.CommonName, orgSpec.CA.Country, orgSpec.CA.Province, orgSpec.CA.Locality, orgSpec.CA.OrganizationalUnit, orgSpec.CA.StreetAddress, orgSpec.CA.PostalCode)
	if err != nil {
		fmt.Printf("Error generating signCA for org %s:\n%v\n", orgName, err)
		os.Exit(1)
	}
	// generate TLS CA
	tlsCA, err := ca2.NewCA(tlsCADir, orgName, "tls"+orgSpec.CA.CommonName, orgSpec.CA.Country, orgSpec.CA.Province, orgSpec.CA.Locality, orgSpec.CA.OrganizationalUnit, orgSpec.CA.StreetAddress, orgSpec.CA.PostalCode)
	if err != nil {
		fmt.Printf("Error generating tlsCA for org %s:\n%v\n", orgName, err)
		os.Exit(1)
	}

	err = msp2.GenerateVerifyingMSP(mspDir, signCA, tlsCA, orgSpec.EnableNodeOUs)
	if err != nil {
		fmt.Printf("Error generating MSP for org %s:\n%v\n", orgName, err)
		os.Exit(1)
	}

	generateNodes(orderersDir, orgSpec.Specs, signCA, tlsCA, msp2.ORDERER, orgSpec.EnableNodeOUs)

	adminUser := NodeSpec{
		isAdmin:    true,
		CommonName: fmt.Sprintf("%s@%s", adminBaseName, orgName),
	}

	// generate an admin for the orderer org
	users := []NodeSpec{}
	// add an admin user
	users = append(users, adminUser)
	generateNodes(usersDir, users, signCA, tlsCA, msp2.CLIENT, orgSpec.EnableNodeOUs)

	// copy the admin cert to the org's MSP admincerts
	if !orgSpec.EnableNodeOUs {
		err = copyAdminCert(usersDir, adminCertsDir, adminUser.CommonName)
		if err != nil {
			fmt.Printf("Error copying admin cert for org %s:\n%v\n",
				orgName, err)
			os.Exit(1)
		}
	}

	// copy the admin cert to each of the org's orderers's MSP admincerts
	for _, spec := range orgSpec.Specs {
		if orgSpec.EnableNodeOUs {
			continue
		}
		err = copyAdminCert(usersDir,
			filepath.Join(orderersDir, spec.CommonName, "msp", "admincerts"), adminUser.CommonName)
		if err != nil {
			fmt.Printf("Error copying admin cert for org %s orderer %s:\n%v\n",
				orgName, spec.CommonName, err)
			os.Exit(1)
		}
	}

}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	cerr := out.Close()
	if err != nil {
		return err
	}
	return cerr
}
