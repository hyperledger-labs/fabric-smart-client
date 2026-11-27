/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chat_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/hyperledger-labs/fabric-smart-client/examples/chat"
	"github.com/hyperledger-labs/fabric-smart-client/integration"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
)

var _ = Describe("EndToEnd", func() {
	Describe("Chat", Label(fsc.WebSocket), func() {
		s := integration.NewTestSuite(func() (*integration.Infrastructure, error) {
			ii, err := integration.New(integration.FSCChatPort.StartPortForNode(), "", chat.Topology(fsc.WebSocket)...)
			if err != nil {
				return nil, err
			}
			ii.Generate()
			return ii, nil
		})
		BeforeEach(s.Setup)
		AfterEach(s.TearDown)

		It("chats in channels and in private", func() { testChat(s.II) })
	})
})

const viewTimeout = time.Minute

// call runs the view registered as fid on node and returns its raw result.
func call(ii *integration.Infrastructure, node, fid string, in any) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), viewTimeout)
	defer cancel()
	res, err := ii.Client(node).CallViewWithContext(ctx, fid, common.JSONMarshall(in))
	if err != nil {
		return nil, err
	}
	raw, ok := res.([]byte)
	Expect(ok).To(BeTrue(), "unexpected view result type %T", res)
	return raw, nil
}

// request runs one server operation from node and returns the server's reply.
func request(ii *integration.Infrastructure, node string, op chat.Op, channel, text string) (*chat.Reply, error) {
	raw, err := call(ii, node, string(op), &chat.Request{Channel: channel, Text: text})
	if err != nil {
		return nil, err
	}
	reply := &chat.Reply{}
	common.JSONUnmarshal(raw, reply)
	return reply, nil
}

func ok(ii *integration.Infrastructure, node string, op chat.Op, channel, text string) *chat.Reply {
	reply, err := request(ii, node, op, channel, text)
	Expect(err).NotTo(HaveOccurred(), "%s %s %s", node, op, channel)
	return reply
}

func inbox(ii *integration.Infrastructure, node string) []chat.Message {
	raw, err := call(ii, node, "inbox", nil)
	Expect(err).NotTo(HaveOccurred())
	var msgs []chat.Message
	common.JSONUnmarshal(raw, &msgs)
	return msgs
}

func dm(ii *integration.Infrastructure, from, to, text string) error {
	_, err := call(ii, from, "dm", &chat.DM{To: to, Text: text})
	return err
}

func testChat(ii *integration.Infrastructure) {
	By("everyone logs in and sees who is online")
	for _, name := range []string{"alice", "bob", "charlie", "dave"} {
		ok(ii, name, chat.OpLogin, "", "")
	}
	Expect(ok(ii, "dave", chat.OpWho, "", "").Users).To(Equal([]chat.User{
		{Name: "alice", Org: "org1"},
		{Name: "bob", Org: "org1"},
		{Name: "charlie", Org: "org2"},
		{Name: "dave", Org: "org2"},
	}))

	By("a post to general reaches every other member, with the sender the server saw")
	for _, name := range []string{"alice", "bob", "charlie"} {
		ok(ii, name, chat.OpJoin, "general", "")
	}
	Expect(ok(ii, "alice", chat.OpPost, "general", "hi").Failed).To(BeEmpty())
	hi := chat.Message{Channel: "general", From: "alice", Text: "hi"}
	Expect(inbox(ii, "bob")).To(Equal([]chat.Message{hi}))
	Expect(inbox(ii, "charlie")).To(Equal([]chat.Message{hi}))
	Expect(inbox(ii, "alice")).To(BeEmpty())

	By("org1 is for org1 only")
	_, err := request(ii, "charlie", chat.OpJoin, "org1", "")
	Expect(err).To(MatchError(ContainSubstring("organization")))
	ok(ii, "alice", chat.OpJoin, "org1", "")
	ok(ii, "bob", chat.OpJoin, "org1", "")
	ok(ii, "bob", chat.OpPost, "org1", "standup in 5")
	Expect(inbox(ii, "alice")).To(Equal([]chat.Message{{Channel: "org1", From: "bob", Text: "standup in 5"}}))
	Expect(inbox(ii, "charlie")).To(BeEmpty())

	By("only members post")
	_, err = request(ii, "dave", chat.OpPost, "general", "let me in")
	Expect(err).To(MatchError(ContainSubstring("not a member")))
	Expect(inbox(ii, "bob")).To(BeEmpty())

	By("a direct message goes from client to client")
	Expect(dm(ii, "alice", "dave", "lunch?")).To(Succeed())
	Expect(inbox(ii, "dave")).To(Equal([]chat.Message{{From: "alice", Text: "lunch?"}}))
	Expect(dm(ii, "alice", "eve", "hello?")).To(MatchError(ContainSubstring("unknown user")))

	By("a member who left gets nothing")
	ok(ii, "bob", chat.OpLeave, "general", "")
	ok(ii, "alice", chat.OpPost, "general", "bye bob")
	Expect(inbox(ii, "bob")).To(BeEmpty())
	Expect(inbox(ii, "charlie")).To(Equal([]chat.Message{{Channel: "general", From: "alice", Text: "bye bob"}}))

	By("a user who logged out leaves every channel, but still gets direct messages")
	ok(ii, "bob", chat.OpLogout, "", "")
	ok(ii, "alice", chat.OpPost, "org1", "anyone?")
	Expect(inbox(ii, "bob")).To(BeEmpty())
	Expect(dm(ii, "alice", "bob", "you there?")).To(Succeed())
	Expect(inbox(ii, "bob")).To(Equal([]chat.Message{{From: "alice", Text: "you there?"}}))

	By("a member whose node hangs delays nobody and is reported to the sender")
	ok(ii, "dave", chat.OpJoin, "general", "")
	ii.PauseFSCNode("charlie")
	defer ii.ResumeFSCNode("charlie") // a failure must not leave the node frozen
	Expect(ok(ii, "alice", chat.OpPost, "general", "anyone awake?").Failed).To(Equal([]string{"charlie"}))
	Expect(inbox(ii, "dave")).To(Equal([]chat.Message{{Channel: "general", From: "alice", Text: "anyone awake?"}}))
	ii.ResumeFSCNode("charlie")

	By("a post reaches everyone it can when a member's node is down")
	ii.StopFSCNode("dave")
	// StopFSCNode only signals the node; wait until it no longer answers.
	Eventually(func() error { _, err := call(ii, "dave", "inbox", nil); return err }, viewTimeout).ShouldNot(Succeed())
	Expect(ok(ii, "alice", chat.OpPost, "general", "still here").Failed).To(Equal([]string{"dave"}))
	// charlie's inbox may also hold the message the server gave up on while charlie hung.
	Expect(inbox(ii, "charlie")).To(ContainElement(chat.Message{Channel: "general", From: "alice", Text: "still here"}))
}
