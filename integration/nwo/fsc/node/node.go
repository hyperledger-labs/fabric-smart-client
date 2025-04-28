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
	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/onsi/gomega"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/api"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"gopkg.in/yaml.v2"
)

type PersistenceKey string

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
	DataSource string
}

type PersistenceOpts struct {
	Type driver.PersistenceType
	SQL  *SQLOpts
}

func (o *Options) PutPersistenceKey(k PersistenceKey) {
	if persistences := o.Get("all_persistence_keys"); persistences != nil {
		o.Put("all_persistence_keys", append(persistences.([]PersistenceKey), k))
	} else {
		o.Put("all_persistence_keys", []PersistenceKey{k})
	}
}

func (o *Options) PutPostgresPersistenceName(k PersistenceKey, p driver2.PersistenceName) {
	o.PutPersistenceKey(k)

	o.Put(fmt.Sprintf("%s.persistence", k), p)
}

func (o *Options) GetAllPersistenceKeys() []PersistenceKey {
	persistences := o.Get("all_persistence_keys")
	if persistences == nil {
		return []PersistenceKey{}
	}
	return persistences.([]PersistenceKey)
}

func (o *Options) GetPersistenceName(k PersistenceKey) (driver2.PersistenceName, bool) {
	if name := o.Get(fmt.Sprintf("%s.persistence", k)); name != nil {
		return name.(driver2.PersistenceName), true
	}
	return "", false
}

func (o *Options) PutPostgresPersistence(k driver2.PersistenceName, p SQLOpts) {
	if persistences := o.Get("postgres_persistences"); persistences != nil {
		o.Put("postgres_persistences", append(persistences.([]driver2.PersistenceName), k))
	} else {
		o.Put("postgres_persistences", []driver2.PersistenceName{k})
	}

	o.Put(fmt.Sprintf("fsc.persistences.%s.persistence.sql", k), p.DataSource)
}

func (o *Options) GetPostgresPersistences() map[driver2.PersistenceName]*SQLOpts {
	persistences := o.Get("postgres_persistences")
	if persistences == nil {
		return nil
	}
	r := make(map[driver2.PersistenceName]*SQLOpts)
	for _, k := range persistences.([]driver2.PersistenceName) {
		r[k] = o.GetPostgresPersistence(k)
	}
	return r
}

func (o *Options) GetPostgresPersistence(k driver2.PersistenceName) *SQLOpts {
	if v := o.Get(fmt.Sprintf("fsc.persistences.%s.persistence.sql", k)); v != nil {
		return &SQLOpts{
			DataSource: v.(string),
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

type SDKElement struct {
	Type  reflect.Type
	Alias string
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

func NewNodeFromTemplate(name string, template *Node) *Node {
	newNode := &Node{
		Synthesizer: Synthesizer{},
	}
	raw, err := json.Marshal(template)
	gomega.Expect(err).ToNot(gomega.HaveOccurred())
	err = json.Unmarshal(raw, newNode)
	gomega.Expect(err).ToNot(gomega.HaveOccurred())
	newNode.Name = name
	newNode.Options = cloneOptions(template.Options)
	return newNode
}

func (n *Node) ReplicaUniqueNames() []string {
	replicationFactor := n.Options.ReplicationFactor()
	names := make([]string, replicationFactor)
	for r := 0; r < replicationFactor; r++ {
		names[r] = ReplicaUniqueName(n.Name, r)
	}
	return names
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

func (n *Node) AddSDKWithBase(base api.SDK, sdks ...api.SDK) *Node {
	elements := make([]SDKElement, len(sdks))

	for idx, sdk := range sdks {
		sdkType := reflect.Indirect(reflect.ValueOf(sdk)).Type()
		alias := n.addImport(sdkType.PkgPath())

		elements[idx] = SDKElement{
			Type:  sdkType,
			Alias: alias,
		}
	}

	// assemble base
	baseType := reflect.Indirect(reflect.ValueOf(base)).Type()
	aliasBase := n.addImport(baseType.PkgPath())
	baseConstruction := fmt.Sprintf("%s.New%s(%s)", aliasBase, baseType.Name(), "n")

	// assemble the sdks recursively, starting from the last
	current := baseConstruction
	for i := len(sdks) - 1; i >= 0; i-- {
		current = fmt.Sprintf("%s.NewFrom(%s)", elements[i].Alias, current)

	}
	n.SDKs = append(n.SDKs, SDKEntry{Type: current})
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
