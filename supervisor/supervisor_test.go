package supervisor

import (
	"fmt"
	"testing"
	"time"

	timetools "github.com/mailgun/vulcand/Godeps/_workspace/src/github.com/mailgun/gotools-time"
	. "github.com/mailgun/vulcand/Godeps/_workspace/src/gopkg.in/check.v1"
	. "github.com/mailgun/vulcand/backend"
	"github.com/mailgun/vulcand/backend/membackend"
	"github.com/mailgun/vulcand/connwatch"
	"github.com/mailgun/vulcand/plugin/registry"
	"github.com/mailgun/vulcand/server"
	. "github.com/mailgun/vulcand/testutils"
)

func TestSupervisor(t *testing.T) { TestingT(t) }

type SupervisorSuite struct {
	tm     *timetools.FreezedTime
	errorC chan error
	sv     *Supervisor
	b      *membackend.MemBackend
}

func (s *SupervisorSuite) SetUpTest(c *C) {

	newServer := func(id int, cw *connwatch.ConnectionWatcher) (server.Server, error) {
		return server.NewMuxServerWithOptions(id, cw, server.Options{})
	}

	s.b = membackend.NewMemBackend(registry.GetRegistry())

	newBackend := func() (Backend, error) {
		return s.b, nil
	}

	s.errorC = make(chan error)

	s.tm = &timetools.FreezedTime{
		CurrentTime: time.Date(2012, 3, 4, 5, 6, 7, 0, time.UTC),
	}

	s.sv = NewSupervisorWithOptions(newServer, newBackend, s.errorC, Options{TimeProvider: s.tm})
}

func (s *SupervisorSuite) TearDownTest(c *C) {
	s.sv.Stop(true)
}

var _ = Suite(&SupervisorSuite{})

func (s *SupervisorSuite) TestStartStopEmpty(c *C) {
	s.sv.Start()
	fmt.Println("Stop")
}

func (s *SupervisorSuite) TestInitFromExistingConfig(c *C) {
	e := NewTestServer("Hi, I'm endpoint")
	defer e.Close()

	l, h := MakeLocation("localhost", "localhost:31000", e.URL)

	_, err := s.b.AddUpstream(l.Upstream)
	c.Assert(err, IsNil)

	_, err = s.b.AddHost(h)
	c.Assert(err, IsNil)

	_, err = s.b.AddLocation(l)
	c.Assert(err, IsNil)

	s.sv.Start()

	c.Assert(GETResponse(c, MakeURL(l, h.Listeners[0]), ""), Equals, "Hi, I'm endpoint")
}

func (s *SupervisorSuite) TestInitOnTheFly(c *C) {
	e := NewTestServer("Hi, I'm endpoint")
	defer e.Close()

	s.sv.Start()

	l, h := MakeLocation("localhost", "localhost:31000", e.URL)

	s.b.ChangesC <- &LocationAdded{
		Host:     h,
		Location: l,
	}

	c.Assert(GETResponse(c, MakeURL(l, h.Listeners[0]), ""), Equals, "Hi, I'm endpoint")
}

func (s *SupervisorSuite) TestGracefulShutdown(c *C) {
	e := NewTestServer("Hi, I'm endpoint")
	defer e.Close()

	s.sv.Start()

	l, h := MakeLocation("localhost", "localhost:31000", e.URL)

	s.b.ChangesC <- &LocationAdded{
		Host:     h,
		Location: l,
	}

	c.Assert(GETResponse(c, MakeURL(l, h.Listeners[0]), ""), Equals, "Hi, I'm endpoint")
	close(s.b.ErrorsC)
}

func (s *SupervisorSuite) TestRestartOnBackendErrors(c *C) {
	e := NewTestServer("Hi, I'm endpoint")
	defer e.Close()

	l, h := MakeLocation("localhost", "localhost:31000", e.URL)

	_, err := s.b.AddUpstream(l.Upstream)
	c.Assert(err, IsNil)

	_, err = s.b.AddHost(h)
	c.Assert(err, IsNil)

	_, err = s.b.AddLocation(l)
	c.Assert(err, IsNil)

	s.sv.Start()

	c.Assert(GETResponse(c, MakeURL(l, h.Listeners[0]), ""), Equals, "Hi, I'm endpoint")
	s.b.ErrorsC <- fmt.Errorf("Restart")

	c.Assert(GETResponse(c, MakeURL(l, h.Listeners[0]), ""), Equals, "Hi, I'm endpoint")
}
