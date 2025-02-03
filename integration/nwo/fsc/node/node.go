/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package node

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/common"
	. "github.com/onsi/gomega"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/api"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"gopkg.in/yaml.v2"
)

// Factory is used to create instances of the View interface
type Factory interface {
	// NewView returns an instance of the View interface build using the passed argument.
	NewView(in []byte) (view.View, error)
}

type Options struct {
	Mapping map[string]interface{}
}

func (o *Options) Parse(opts ...Option) error {
	for _, opt := range opts {
		if err := opt(o); err != nil {
			return err
		}
	}

	return nil
}

func (o *Options) Put(k string, v interface{}) {
	if o.Mapping == nil {
		o.Mapping = map[string]interface{}{}
	}
	o.Mapping[k] = v
}

func (o *Options) Get(k string) interface{} {
	if o.Mapping == nil {
		return nil
	}
	return o.Mapping[k]
}

type SQLOpts struct {
	DataSource   string
	DriverType   common.SQLDriverType
	CreateSchema bool
	TablePrefix  string
	MaxOpenConns int
}

type PersistenceOpts struct {
	Type driver.PersistenceType
	SQL  *SQLOpts
}

func (o *Options) PutSQLPersistence(k string, p SQLOpts) {
	if persistences := o.Get("persistences"); persistences != nil {
		o.Put("persistences", append(persistences.([]string), k))
	} else {
		o.Put("persistences", []string{k})
	}

	o.Put(k+".persistence.sql", p.DataSource)
	o.Put(k+".persistence.driver", p.DriverType)
	o.Put(k+"persistence.createSchema", p.CreateSchema)
	o.Put(k+"persistence.tablePrefix", p.TablePrefix)
	o.Put(k+"persistence.maxOpenConns", p.MaxOpenConns)
}

func (o *Options) GetPersistences() map[string]*SQLOpts {
	persistences := o.Get("persistences")
	if persistences == nil {
		return nil
	}
	r := make(map[string]*SQLOpts)
	for _, prefix := range persistences.([]string) {
		r[prefix] = o.GetPersistence(prefix)
	}
	return r
}

func (o *Options) GetPersistence(k string) *SQLOpts {
	if v := o.Get(k + ".persistence.sql"); v != nil {
		return &SQLOpts{
			DataSource:   v.(string),
			DriverType:   common.SQLDriverType(utils.DefaultString(o.Get(k+".persistence.driver"), string(sql.Postgres))),
			CreateSchema: utils.DefaultZero[bool](o.Get(k + ".persistence.createSchema")),
			TablePrefix:  utils.DefaultZero[string](o.Get(k + ".persistence.tablePrefix")),
			MaxOpenConns: utils.DefaultInt(o.Get(k+".persistence.maxOpenConns"), 200),
		}
	}
	return nil
}

func (o *Options) ReplicationFactor() int {
	if f, ok := o.Mapping["Replication"]; ok && f.(int) > 0 {
		return f.(int)
	}
	return 1
}

func (o *Options) Aliases() []string {
	boxed := o.Mapping["Aliases"]
	if boxed == nil {
		return nil
	}
	res, ok := boxed.([]string)
	if ok {
		return res
	}
	res = []string{}
	for _, v := range boxed.([]interface{}) {
		res = append(res, v.(string))
	}
	return res
}

func (o *Options) AddAlias(alias string) {
	if o.Mapping == nil {
		o.Mapping = map[string]interface{}{}
	}
	aliasesBoxed, ok := o.Mapping["Aliases"]
	if !ok {
		o.Mapping["Aliases"] = []string{alias}
		return
	}
	aliases, ok := aliasesBoxed.([]string)
	if ok {
		aliases = append(aliases, alias)
		o.Mapping["Aliases"] = aliases
		return
	}

	for _, v := range aliasesBoxed.([]interface{}) {
		aliases = append(aliases, v.(string))
	}
	aliases = append(aliases, alias)
	o.Mapping["Aliases"] = aliases
}

type Option func(*Options) error

type FactoryEntry struct {
	Id   string
	Type string
}

type ResponderEntry struct {
	Responder string
	Initiator string
}

type SDKEntry struct {
	Id   string
	Type string
}

type Alias struct {
	Original string
	Alias    string
}

type Synthesizer struct {
	Aliases    map[string]Alias `yaml:"Aliases,omitempty"`
	Imports    []string         `yaml:"Imports,omitempty"`
	Factories  []FactoryEntry   `yaml:"Factories,omitempty"`
	SDKs       []SDKEntry       `yaml:"SDKs,omitempty"`
	Responders []ResponderEntry `yaml:"Responders,omitempty"`
}

type Node struct {
	Synthesizer    `yaml:"Synthesizer,omitempty"`
	Name           string   `yaml:"name,omitempty"`
	Bootstrap      bool     `yaml:"bootstrap,omitempty"`
	ExecutablePath string   `yaml:"executablePath,omitempty"`
	Path           string   `yaml:"path,omitempty"`
	Options        *Options `yaml:"options,omitempty"`
}

func NewNode(name string) *Node {
	return &Node{
		Synthesizer: Synthesizer{
			Aliases:    map[string]Alias{},
			Imports:    []string{},
			Factories:  []FactoryEntry{},
			Responders: []ResponderEntry{},
			SDKs:       []SDKEntry{},
		},
		Name:    name,
		Options: &Options{Mapping: map[string]interface{}{}},
	}
}

func (n *Node) ReplicaUniqueNames() []string {
	replicationFactor := n.Options.ReplicationFactor()
	names := make([]string, replicationFactor)
	for r := 0; r < replicationFactor; r++ {
		names[r] = ReplicaUniqueName(n.Name, r)
	}
	return names
}

func NewNodeFromTemplate(name string, template *Node) *Node {
	newNode := &Node{
		Synthesizer: Synthesizer{},
	}
	raw, err := json.Marshal(template)
	Expect(err).ToNot(HaveOccurred())
	err = json.Unmarshal(raw, newNode)
	Expect(err).ToNot(HaveOccurred())
	newNode.Name = name
	newNode.Options = cloneOptions(template.Options)
	return newNode
}

func (n *Node) ID() string {
	return n.Name
}

func (n *Node) SetBootstrap() *Node {
	n.Bootstrap = true

	return n
}

// SetExecutable sets the executable path of this node
func (n *Node) SetExecutable(ExecutablePath string) *Node {
	n.ExecutablePath = ExecutablePath

	return n
}

// AddSDK adds a reference to the passed SDK. AddSDK expects to find a constructor named
// 'New' followed by the type name of the passed reference.
func (n *Node) AddSDK(sdk api.SDK) *Node {
	sdkType := reflect.Indirect(reflect.ValueOf(sdk)).Type()

	alias := n.addImport(sdkType.PkgPath())
	sdkStr := alias + ".New" + sdkType.Name() + "(n)"

	n.SDKs = append(n.SDKs, SDKEntry{Type: sdkStr})

	return n
}

func (n *Node) RegisterViewFactory(id string, factory Factory) *Node {
	isFactoryPtr := reflect.ValueOf(factory).Kind() == reflect.Ptr
	factoryType := reflect.Indirect(reflect.ValueOf(factory)).Type()

	alias := n.addImport(factoryType.PkgPath())
	factoryStr := ""
	if isFactoryPtr {
		factoryStr += "&"
	}
	factoryStr += alias + "." + factoryType.Name() + "{}"

	n.Factories = append(n.Factories, FactoryEntry{Id: id, Type: factoryStr})

	return n
}

// RegisterResponder registers the passed responder to the passed initiator
func (n *Node) RegisterResponder(responder view.View, initiator view.View) *Node {
	isResponderPtr := reflect.ValueOf(responder).Kind() == reflect.Ptr
	isInitiatorPtr := reflect.ValueOf(initiator).Kind() == reflect.Ptr
	responderType := reflect.Indirect(reflect.ValueOf(responder)).Type()
	initiatorType := reflect.Indirect(reflect.ValueOf(initiator)).Type()

	aliasResponder := n.addImport(responderType.PkgPath())
	aliasInitiator := n.addImport(initiatorType.PkgPath())

	responderStr := ""
	if isResponderPtr {
		responderStr += "&"
	}
	responderStr += aliasResponder + "." + responderType.Name() + "{}"

	initiatorStr := ""
	if isInitiatorPtr {
		initiatorStr += "&"
	}
	initiatorStr += aliasInitiator + "." + initiatorType.Name() + "{}"

	n.Responders = append(n.Responders, ResponderEntry{Responder: responderStr, Initiator: initiatorStr})

	return n
}

func (n *Node) AddOptions(opts ...Option) *Node {
	if err := n.Options.Parse(opts...); err != nil {
		panic(err.Error())
	}
	return n
}

func (n *Node) PlatformOpts() *Options {
	return n.Options
}

func (n *Node) Alias(i string) string {
	return n.Aliases[i].Alias
}

func (n *Node) addImport(i string) string {
	index := sort.SearchStrings(n.Imports, i)
	if index < len(n.Imports) && n.Imports[index] == i {
		return n.Aliases[i].Alias
	}

	elements := strings.SplitAfter(i, "/")
	newAlias := elements[len(elements)-1]
	counter := 0
	for _, alias := range n.Aliases {
		if alias.Original == newAlias {
			counter++
		}
	}
	if counter > 0 {
		newAlias += strconv.Itoa(counter)
	}
	n.Aliases[i] = Alias{
		Original: elements[len(elements)-1],
		Alias:    newAlias,
	}

	var imports []string
	imports = append(imports, n.Imports[:index]...)
	imports = append(imports, i)
	imports = append(imports, n.Imports[index:]...)
	n.Imports = imports

	return n.Aliases[i].Alias
}

func cloneOptions(options *Options) *Options {
	// deep clone options using yaml
	b, err := yaml.Marshal(options)
	if err != nil {
		panic(err.Error())
	}
	var clone Options
	err = yaml.Unmarshal(b, &clone)
	if err != nil {
		panic(err.Error())
	}
	return &clone
}

func ReplicaUniqueName(nodeName string, r int) string {
	return fmt.Sprintf("%s.%d", nodeName, r)
}
