/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package simple_test

import (
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/integration"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabricx/simple"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabricx/simple/views"
	nwocommon "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common"
	nwofabricx "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabricx"
	nwofsc "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const testDataPath = "./out/testdata"

func TestEndToEnd(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "simple suite")
}

type TestSuite struct {
	*integration.TestSuite
}

var _ = Describe("EndToEnd", func() {
	for _, c := range []nwofsc.P2PCommunicationType{nwofsc.WebSocket} {
		Describe("simple Life Cycle", Label(c), func() {
			s := NewTestSuite(c)
			BeforeEach(s.Setup)
			AfterEach(s.TearDown)
			It("succeeded", func() {
				RunTest(s.II)
			})
		})
	}
})

var testObjects = []views.SomeObject{
	{
		Owner: "Boss",
		Value: 100,
	},
	{
		Owner: "Bob",
		Value: 200,
	},
	{
		Owner: "RedHotChilli",
		Value: 900,
	},
	{
		Owner: "DogsEatCarrots",
		Value: 600,
	},
}

// This object has a negative value, it should return an error from the test.
var negativeTestObjects = []views.SomeObject{
	{
		Owner: "TigerEatsGrass",
		Value: -100,
	},
}

func RunTest(n *integration.Infrastructure) {
	// create some objects
	for _, obj := range testObjects {
		res, err := n.Client(simple.CreatorNode).CallView("create", nwocommon.JSONMarshall(views.CreateParams{
			Owner:     obj.Owner,
			Value:     obj.Value,
			Namespace: simple.Namespace,
			Approvers: []view.Identity{n.Identity(simple.ApproverNode)},
		}))
		Expect(err).NotTo(HaveOccurred())
		_ = res
	}

	// try to create one of our objects again, this should fail and return with an error
	someExistingObject := testObjects[0]
	_, err := n.Client(simple.CreatorNode).CallView("create", nwocommon.JSONMarshall(views.CreateParams{
		Owner:     someExistingObject.Owner,
		Value:     someExistingObject.Value,
		Namespace: simple.Namespace,
		Approvers: []view.Identity{n.Identity(simple.ApproverNode)},
	}))
	Expect(err).To(HaveOccurred())

	// try to create an object with a negative value, this should fail and return with an error
	someAbsurdObject := negativeTestObjects[0]
	_, err = n.Client(simple.CreatorNode).CallView("create", nwocommon.JSONMarshall(views.CreateParams{
		Owner:     someAbsurdObject.Owner,
		Value:     someAbsurdObject.Value,
		Namespace: simple.Namespace,
		Approvers: []view.Identity{n.Identity(simple.ApproverNode)},
	}))
	Expect(err).To(HaveOccurred())

	// lets see that we can query all our objects
	lookupIDs := make([]string, len(testObjects))
	for i, obj := range testObjects {
		linearID, errx := obj.GetLinearID()
		Expect(errx).ToNot(HaveOccurred())
		lookupIDs[i] = linearID
	}

	res, err := n.Client(simple.CreatorNode).CallView("query", nwocommon.JSONMarshall(views.QueryParams{
		SomeIDs:   lookupIDs,
		Namespace: simple.Namespace,
	}))
	Expect(err).ToNot(HaveOccurred())

	raw, ok := res.([]byte)
	Expect(ok).To(BeTrue())
	var objs []views.SomeObject
	nwocommon.JSONUnmarshal(raw, &objs)
	Expect(objs).To(ConsistOf(testObjects))

	// do some lookup which do not exist
	_, err = n.Client(simple.CreatorNode).CallView("query", nwocommon.JSONMarshall(views.QueryParams{
		SomeIDs:   []string{"doo", "boo", "bong", "ding"},
		Namespace: simple.Namespace,
	}))
	Expect(err).ToNot(HaveOccurred())
}

func NewTestSuite(commType nwofsc.P2PCommunicationType) *TestSuite {
	return &TestSuite{integration.NewTestSuite(func() (*integration.Infrastructure, error) {
		ii, err := integration.New(integration.IOUPort.StartPortForNode(), testDataPath, simple.Topology(&simple.SDK{}, commType)...)
		if err != nil {
			return nil, err
		}

		ii.RegisterPlatformFactory(nwofabricx.NewPlatformFactory())

		ii.DeleteOnStart = true
		ii.DeleteOnStop = false

		ii.Generate()

		return ii, nil
	})}
}
