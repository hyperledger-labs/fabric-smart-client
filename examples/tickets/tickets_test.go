/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package tickets_test

import (
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/hyperledger-labs/fabric-smart-client/examples/tickets"
	"github.com/hyperledger-labs/fabric-smart-client/examples/tickets/api"
	"github.com/hyperledger-labs/fabric-smart-client/examples/tickets/states"
	"github.com/hyperledger-labs/fabric-smart-client/examples/tickets/views"
	"github.com/hyperledger-labs/fabric-smart-client/integration"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common"
	nwofabricx "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabricx"
	nwofsc "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
	viewclient "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/client"
)

var _ = Describe("EndToEnd", func() {
	Describe("Ticket booking", Label(nwofsc.WebSocket), Ordered, func() {
		s := NewTestSuite(nwofsc.WebSocket)
		BeforeAll(s.Setup)
		AfterAll(s.TearDown)

		It("sells a seat once and reports the race", s.TestRace)
		It("serialises concurrent creates", s.TestConcurrentCreates)
		It("reports a price change during a held buy as a conflict", s.TestPriceChange)
		It("rejects calls without the node's certificate", s.TestCertificates)
		It("keeps promoter routes off resellers", s.TestRoutes)
		It("serves a web UI on every node", s.TestUI)
	})
})

type TestSuite struct {
	*integration.TestSuite
}

func NewTestSuite(commType nwofsc.P2PCommunicationType) *TestSuite {
	return &TestSuite{integration.NewTestSuite(func() (*integration.Infrastructure, error) {
		ii, err := integration.New(integration.FabricXTicketsPort.StartPortForNode(), "", tickets.Topology(&tickets.SDK{}, commType)...)
		if err != nil {
			return nil, err
		}
		ii.RegisterPlatformFactory(nwofabricx.NewPlatformFactory())
		ii.Generate()
		return ii, nil
	})}
}

// hold keeps an endorsed sale from being submitted for most of views.MaxHold,
// so a second buy or a price change surely lands inside it, even on a slow
// runner.
const hold = 8 * time.Second

func sampleEvent(name string) states.Event {
	return states.Event{Name: name, Artist: "Neon", Venue: "Berlin", Date: "2027-03-02", Seats: 40, Price: 50}
}

func (s *TestSuite) create(e states.Event) *states.Event {
	status, body := newRESTClient(s.II, views.PromoterNode).do(http.MethodPost, "/api/events", e)
	Expect(status).To(Equal(http.StatusCreated), string(body))
	out := &states.Event{}
	common.JSONUnmarshal(body, out)
	Expect(out.ID).NotTo(BeEmpty())
	return out
}

// buy returns the status and, on success, the seat; on failure, the error code.
func (s *TestSuite) buy(reseller, eventID string, seat int, hold time.Duration) (int, *states.Seat, string) {
	path := fmt.Sprintf("/api/events/%s/seats/%d?hold=%s", eventID, seat, hold)
	status, body := newRESTClient(s.II, reseller).do(http.MethodPost, path, nil)
	if status != http.StatusCreated {
		return status, nil, errorCode(body)
	}
	out := &states.Seat{}
	common.JSONUnmarshal(body, out)
	return status, out, ""
}

func (s *TestSuite) event(node, id string) *api.Event {
	status, body := newRESTClient(s.II, node).do(http.MethodGet, "/api/events/"+id, nil)
	Expect(status).To(Equal(http.StatusOK), string(body))
	out := &api.Event{}
	common.JSONUnmarshal(body, out)
	return out
}

func (s *TestSuite) TestRace() {
	e := s.create(sampleEvent("Neon Nights"))

	By("both resellers read the event from the ledger")
	for _, r := range tickets.Resellers {
		Expect(s.event(r, e.ID).Name).To(Equal("Neon Nights"))
	}

	By("reseller-eu sells seat 1")
	status, seat, _ := s.buy("reseller-eu", e.ID, 1, 0)
	Expect(status).To(Equal(http.StatusCreated))
	Expect(seat.Reseller).To(Equal("reseller-eu"))
	Expect(seat.Price).To(BeEquivalentTo(50))
	Expect(seat.Ticket).To(HaveLen(32))

	By("reseller-us cannot sell it again")
	status, _, code := s.buy("reseller-us", e.ID, 1, 0)
	Expect(status).To(Equal(http.StatusConflict))
	Expect(code).To(Equal("already_sold"))

	By("both resellers sell seat 2 at once, holding after the endorsement")
	var wg sync.WaitGroup
	statuses := make([]int, len(tickets.Resellers))
	codes := make([]string, len(tickets.Resellers))
	for i, r := range tickets.Resellers {
		wg.Go(func() {
			defer GinkgoRecover()
			statuses[i], _, codes[i] = s.buy(r, e.ID, 2, hold)
		})
	}
	wg.Wait()
	Expect(statuses).To(ConsistOf(http.StatusCreated, http.StatusConflict), "codes: %v", codes)
	Expect(codes).To(ConsistOf("", "lost_race"))

	By("the promoter sees seats 1 and 2 sold")
	got := s.event(views.PromoterNode, e.ID)
	Expect(got.Sold["reseller-eu"] + got.Sold["reseller-us"]).To(Equal(2))
	Expect(got.Seating).To(HaveLen(40))
	Expect(got.Seating[0].Sold).To(BeTrue())
	Expect(got.Seating[1].Sold).To(BeTrue())
	Expect(got.Seating[2].Sold).To(BeFalse())
}

func (s *TestSuite) TestConcurrentCreates() {
	var wg sync.WaitGroup
	names := []string{"Parallel A", "Parallel B"}
	for _, name := range names {
		wg.Go(func() {
			defer GinkgoRecover()
			s.create(sampleEvent(name))
		})
	}
	wg.Wait()

	status, body := newRESTClient(s.II, "reseller-us").do(http.MethodGet, "/api/events", nil)
	Expect(status).To(Equal(http.StatusOK))
	var list []*api.Event
	common.JSONUnmarshal(body, &list)
	var got []string
	for _, e := range list {
		got = append(got, e.Name)
	}
	Expect(got).To(ContainElements(names))
}

func (s *TestSuite) TestPriceChange() {
	e := s.create(sampleEvent("Price Change"))

	codes := make(chan string, 1)
	go func() {
		defer GinkgoRecover()
		code := "buy failed"
		defer func() { codes <- code }()
		_, _, code = s.buy("reseller-eu", e.ID, 3, hold)
	}()
	// The buy is endorsed well within this, and then held for the rest of hold.
	time.Sleep(3 * time.Second)
	updated := *e
	updated.Price = 60
	status, body := newRESTClient(s.II, views.PromoterNode).do(http.MethodPut, "/api/events/"+e.ID, updated)
	Expect(status).To(Equal(http.StatusOK), string(body))

	Expect(<-codes).To(Equal("conflict"))

	By("seat 3 is still free and sells at the new price")
	status, seat, _ := s.buy("reseller-eu", e.ID, 3, 0)
	Expect(status).To(Equal(http.StatusCreated))
	Expect(seat.Price).To(BeEquivalentTo(60))
}

func (s *TestSuite) TestCertificates() {
	By("a client without a certificate is turned away")
	anon := newCertlessClient(s.II, "reseller-eu")
	// The listener asks for a client certificate but does not demand one, so
	// the handshake succeeds and the API's mTLS check answers.
	resp, err := anon.Get(anon.URL + "/api/events")
	Expect(err).NotTo(HaveOccurred())
	_ = resp.Body.Close()
	Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))

	By("reseller-us's certificate does not open reseller-eu's API")
	cc := connection(s.II, "reseller-eu")
	other := connection(s.II, "reseller-us")
	cc.TLS.Certificate, cc.TLS.Key = other.TLS.Certificate, other.TLS.Key
	c, err := viewclient.NewHTTPClient(cc)
	Expect(err).NotTo(HaveOccurred())
	us := &restClient{HTTPClient: c}
	status, body := us.do(http.MethodGet, "/api/events", nil)
	Expect(status).To(Equal(http.StatusForbidden))
	Expect(errorCode(body)).To(Equal("forbidden"))
}

func (s *TestSuite) TestRoutes() {
	status, _ := newRESTClient(s.II, "reseller-eu").do(http.MethodPost, "/api/events", sampleEvent("Not here"))
	Expect(status).To(BeElementOf(http.StatusNotFound, http.StatusMethodNotAllowed))
}

func (s *TestSuite) TestUI() {
	e := s.create(sampleEvent("UI Night"))
	for node, want := range map[string]string{
		views.PromoterNode: "New event",
		"reseller-eu":      `formaction="/ui/events/` + e.ID + `/seats/1"`,
	} {
		// The UI needs no client certificate.
		c := newCertlessClient(s.II, node)
		resp, err := c.Get(c.URL + "/?event=" + e.ID)
		Expect(err).NotTo(HaveOccurred())
		body, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(http.StatusOK), node)
		Expect(string(body)).To(ContainSubstring("UI Night"), node)
		Expect(string(body)).To(ContainSubstring(want), node)
	}

	By("the stylesheet is served from the node")
	c := newCertlessClient(s.II, "reseller-us")
	resp, err := c.Get(c.URL + "/static/beer.min.css")
	Expect(err).NotTo(HaveOccurred())
	_ = resp.Body.Close()
	Expect(resp.StatusCode).To(Equal(http.StatusOK))
}
