package configure

import (
	"github.com/mailgun/vulcan"
	"github.com/mailgun/vulcan/limit/tokenbucket"
	"github.com/mailgun/vulcan/loadbalance/roundrobin"
	"github.com/mailgun/vulcan/route/hostroute"
	"github.com/mailgun/vulcan/route/pathroute"
	. "github.com/mailgun/vulcand/backend"
	"github.com/mailgun/vulcand/plugin/ratelimit"
	. "launchpad.net/gocheck"
	"testing"
	"time"
)

func TestConfigure(t *testing.T) { TestingT(t) }

type ConfSuite struct {
	router *hostroute.HostRouter
	proxy  *vulcan.Proxy
	conf   *Configurator
}

func (s *ConfSuite) SetUpTest(c *C) {
	s.router = hostroute.NewHostRouter()
	proxy, err := vulcan.NewProxy(s.router)
	if err != nil {
		c.Fatal(err)
	}
	s.conf = NewConfigurator(proxy)
}

var _ = Suite(&ConfSuite{})

func (s *ConfSuite) AssertSameEndpoints(c *C, a []*roundrobin.WeightedEndpoint, b []*Endpoint) {
	x, y := map[string]bool{}, map[string]bool{}
	for _, e := range a {
		x[e.GetUrl().String()] = true
	}

	for _, e := range b {
		y[e.Url] = true
	}
	c.Assert(x, DeepEquals, y)
}

func (s *ConfSuite) makeRateLimit(id string, rate int, variable string, burst int, periodSeconds int, loc *Location) *MiddlewareInstance {
	rl, err := ratelimit.NewRateLimit(rate, variable, burst, periodSeconds)
	if err != nil {
		panic(err)
	}
	return &MiddlewareInstance{
		Type:       "ratelimit",
		Id:         id,
		Middleware: rl,
	}
}

func (s *ConfSuite) TestUnsupportedChange(c *C) {
	err := s.conf.processChange(nil)
	c.Assert(err, NotNil)
}

func (s *ConfSuite) TestAddDeleteHost(c *C) {
	host := &Host{Name: "localhost"}

	err := s.conf.processChange(&HostAdded{Host: host})
	c.Assert(err, IsNil)

	r := s.conf.a.GetPathRouter(host.Name)
	c.Assert(r, NotNil)

	err = s.conf.processChange(&HostDeleted{Name: host.Name})
	c.Assert(err, IsNil)

	r = s.conf.a.GetPathRouter(host.Name)
	c.Assert(r, IsNil)
}

func (s *ConfSuite) TestAddDeleteLocation(c *C) {
	host := &Host{Name: "localhost"}
	upstream := &Upstream{
		Id: "up1",
		Endpoints: []*Endpoint{
			{
				Url: "http://localhost:5000",
			},
		},
	}
	location := &Location{
		Hostname: host.Name,
		Path:     "/home",
		Id:       "loc1",
		Upstream: upstream,
	}

	location.Middlewares = []*MiddlewareInstance{
		s.makeRateLimit("rl1", 100, "client.ip", 200, 10, location),
	}

	err := s.conf.processChange(&LocationAdded{Host: host, Location: location})
	c.Assert(err, IsNil)

	// Make sure location is here
	l := s.conf.a.GetHttpLocation(host.Name, location.Id)
	c.Assert(l, NotNil)

	// Make sure the endpoint has been added to the location
	lb := s.conf.a.GetHttpLocationLb(host.Name, location.Id)
	c.Assert(lb, NotNil)

	// Check that endpoint is here
	endpoints := lb.GetEndpoints()
	c.Assert(len(endpoints), Equals, 1)
	s.AssertSameEndpoints(c, endpoints, upstream.Endpoints)

	// Make sure connection limit and rate limit are here as well
	chain := l.GetMiddlewareChain()
	c.Assert(chain.Get("ratelimit.rl1"), NotNil)

	// Delete the location
	err = s.conf.processChange(&LocationDeleted{Host: host, LocationId: location.Id})
	c.Assert(err, IsNil)

	// Make sure it's no longer in the proxy
	l = s.conf.a.GetHttpLocation(host.Name, location.Id)
	c.Assert(l, IsNil)
}

func (s *ConfSuite) TestAddLocationTwice(c *C) {
	location, host := makeLocation()

	err := s.conf.processChange(&LocationAdded{Host: host, Location: location})
	c.Assert(err, IsNil)

	err = s.conf.processChange(&LocationAdded{Host: host, Location: location})
	c.Assert(err, IsNil)
}

func (s *ConfSuite) TestUpdateLocationUpstream(c *C) {
	host := &Host{Name: "localhost"}
	up1 := &Upstream{
		Id: "up1",
		Endpoints: []*Endpoint{
			{
				Url: "http://localhost:5000",
			},
			{
				Url: "http://localhost:5001",
			},
		},
	}

	up2 := &Upstream{
		Id: "up2",
		Endpoints: []*Endpoint{
			{
				Url: "http://localhost:5001",
			},
			{
				Url: "http://localhost:5002",
			},
		},
	}

	location := &Location{
		Hostname: host.Name,
		Path:     "/home",
		Id:       "loc1",
		Upstream: up1,
	}

	err := s.conf.processChange(&LocationAdded{Host: host, Location: location})
	c.Assert(err, IsNil)

	// Make sure the endpoint has been added to the location
	lb := s.conf.a.GetHttpLocationLb(host.Name, location.Id)
	c.Assert(lb, NotNil)

	// Endpoints are taken from up1
	s.AssertSameEndpoints(c, lb.GetEndpoints(), up1.Endpoints)

	location.Upstream = up2
	err = s.conf.processChange(
		&LocationUpstreamUpdated{Host: host, Location: location})
	c.Assert(err, IsNil)

	// Endpoints are taken from up2
	s.AssertSameEndpoints(c, lb.GetEndpoints(), up2.Endpoints)
}

func (s *ConfSuite) TestUpstreamAddEndpoint(c *C) {
	location, host := makeLocation()
	up := location.Upstream

	err := s.conf.processChange(&LocationAdded{Host: host, Location: location})
	c.Assert(err, IsNil)

	// Make sure the endpoint has been added to the location
	lb := s.conf.a.GetHttpLocationLb(host.Name, location.Id)
	c.Assert(lb, NotNil)

	// Endpoints are taken from the upstream
	s.AssertSameEndpoints(c, lb.GetEndpoints(), up.Endpoints)

	// Add some endpoints to location
	newEndpoint := &Endpoint{
		Url: "http://localhost:5008",
	}
	up.Endpoints = append(up.Endpoints, newEndpoint)

	err = s.conf.processChange(&EndpointAdded{Upstream: up, Endpoint: newEndpoint, AffectedLocations: []*Location{location}})
	c.Assert(err, IsNil)

	// Endpoints have propagated
	s.AssertSameEndpoints(c, lb.GetEndpoints(), up.Endpoints)
}

func (s *ConfSuite) TestUpstreamBadAddEndpoint(c *C) {
	location, host := makeLocation()
	up := location.Upstream

	err := s.conf.processChange(&LocationAdded{Host: host, Location: location})
	c.Assert(err, IsNil)

	// Make sure the endpoint has been added to the location
	lb := s.conf.a.GetHttpLocationLb(host.Name, location.Id)
	c.Assert(lb, NotNil)

	// Add some endpoints to location
	newEndpoint := &Endpoint{
		Url: "http: local-host :500",
	}
	up.Endpoints = append(up.Endpoints, newEndpoint)

	err = s.conf.processChange(&EndpointAdded{Upstream: up, Endpoint: newEndpoint, AffectedLocations: []*Location{location}})
	c.Assert(err, NotNil)

	// Endpoints haven't been affected
	s.AssertSameEndpoints(c, lb.GetEndpoints(), up.Endpoints[:1])
}

func (s *ConfSuite) TestUpstreamDeleteEndpoint(c *C) {
	location, host := makeLocation()
	up := location.Upstream

	err := s.conf.processChange(&LocationAdded{Host: host, Location: location})
	c.Assert(err, IsNil)

	e := up.Endpoints[0]
	up.Endpoints = []*Endpoint{}

	err = s.conf.processChange(&EndpointDeleted{
		Upstream:          up,
		EndpointId:        e.Id,
		AffectedLocations: []*Location{location},
	})
	c.Assert(err, IsNil)

	lb := s.conf.a.GetHttpLocationLb(host.Name, location.Id)
	c.Assert(lb, NotNil)
	s.AssertSameEndpoints(c, lb.GetEndpoints(), up.Endpoints)
}

func (s *ConfSuite) TestUpstreamUpdateEndpoint(c *C) {
	location, host := makeLocation()
	up := location.Upstream

	err := s.conf.processChange(&LocationAdded{Host: host, Location: location})
	c.Assert(err, IsNil)

	e := up.Endpoints[0]
	e.Url = "http://localhost:7000"

	err = s.conf.processChange(&EndpointUpdated{Upstream: up, Endpoint: e, AffectedLocations: []*Location{location}})
	c.Assert(err, IsNil)

	lb := s.conf.a.GetHttpLocationLb(host.Name, location.Id)
	c.Assert(lb, NotNil)
	s.AssertSameEndpoints(c, lb.GetEndpoints(), up.Endpoints)
}

func (s *ConfSuite) TestAddRemoveUpstreams(c *C) {
	location, _ := makeLocation()
	up := location.Upstream

	c.Assert(s.conf.processChange(&UpstreamAdded{up}), IsNil)
	c.Assert(s.conf.processChange(&UpstreamDeleted{UpstreamId: up.Id}), IsNil)
}

func (s *ConfSuite) TestUpdateRateLimit(c *C) {
	location, host := makeLocation()

	err := s.conf.processChange(&LocationAdded{Host: host, Location: location})
	c.Assert(err, IsNil)

	rl := s.makeRateLimit("rl1", 100, "client.ip", 200, 10, location)

	err = s.conf.processChange(&LocationMiddlewareAdded{Host: host, Location: location, Middleware: rl})
	c.Assert(err, IsNil)

	l := s.conf.a.GetHttpLocation(host.Name, location.Id)
	c.Assert(l, NotNil)

	// Make sure connection limit and rate limit are here as well
	chain := l.GetMiddlewareChain()
	limiter := chain.Get("ratelimit.rl1").(*tokenbucket.TokenLimiter)
	c.Assert(limiter.GetRate().Units, Equals, int64(100))
	c.Assert(limiter.GetRate().Period, Equals, time.Second*time.Duration(10))
	c.Assert(limiter.GetBurst(), Equals, 200)

	// Update the rate limit
	rl = s.makeRateLimit("rl1", 12, "client.ip", 20, 3, location)
	err = s.conf.processChange(&LocationMiddlewareUpdated{Host: host, Location: location, Middleware: rl})
	c.Assert(err, IsNil)

	// Make sure the changes have taken place
	limiter = chain.Get("ratelimit.rl1").(*tokenbucket.TokenLimiter)
	c.Assert(limiter.GetRate().Units, Equals, int64(12))
	c.Assert(limiter.GetRate().Period, Equals, time.Second*time.Duration(3))
	c.Assert(limiter.GetBurst(), Equals, 20)
}

func (s *ConfSuite) TestAddDeleteRateLimit(c *C) {
	location, host := makeLocation()

	err := s.conf.processChange(&LocationAdded{Host: host, Location: location})
	c.Assert(err, IsNil)

	rl := s.makeRateLimit("r1", 10, "client.ip", 1, 1, location)
	rl2 := s.makeRateLimit("r2", 10, "client.ip", 1, 1, location)

	err = s.conf.processChange(&LocationMiddlewareAdded{Host: host, Location: location, Middleware: rl})
	c.Assert(err, IsNil)

	err = s.conf.processChange(&LocationMiddlewareAdded{Host: host, Location: location, Middleware: rl2})
	c.Assert(err, IsNil)

	l := s.conf.a.GetHttpLocation(host.Name, location.Id)
	c.Assert(err, IsNil)
	c.Assert(l, NotNil)

	chain := l.GetMiddlewareChain()
	c.Assert(chain.Get("ratelimit.r1"), NotNil)
	c.Assert(chain.Get("ratelimit.r2"), NotNil)

	err = s.conf.processChange(
		&LocationMiddlewareDeleted{
			Host:           host,
			Location:       location,
			MiddlewareId:   rl.Id,
			MiddlewareType: rl.Type,
		})
	c.Assert(err, IsNil)
	c.Assert(chain.Get("ratelimit.r1"), IsNil)
	// Make sure that the other rate limiter is still there
	c.Assert(chain.Get("ratelimit.r2"), NotNil)
}

func (s *ConfSuite) TestUpdateLocationPath(c *C) {
	location, host := makeLocation()

	err := s.conf.processChange(&LocationAdded{Host: host, Location: location})
	c.Assert(err, IsNil)

	// Host router matches inner router by hostname
	router := s.conf.a.GetHostRouter().GetRouter(host.Name)
	c.Assert(router, NotNil)
	pathRouter := router.(*pathroute.PathRouter)

	// Make sure that path router is configured correctly
	l := pathRouter.GetLocationByPattern(location.Path)
	c.Assert(l, NotNil)

	// Update location path
	oldPath := location.Path
	location.Path = "/new/path"
	err = s.conf.processChange(&LocationPathUpdated{Host: host, Location: location})
	c.Assert(err, IsNil)

	l = pathRouter.GetLocationByPattern(oldPath)
	c.Assert(l, IsNil)

	l = pathRouter.GetLocationByPattern(location.Path)
	c.Assert(l, NotNil)
}

// Make sure that update location path will actually create a location if it does not exist
func (s *ConfSuite) TestUpdateLocationPathUpsertsLocation(c *C) {
	location, host := makeLocation()

	err := s.conf.processChange(&LocationPathUpdated{Host: host, Location: location})
	c.Assert(err, IsNil)

	router := s.conf.a.GetHostRouter().GetRouter(host.Name)
	c.Assert(router, NotNil)
	pathRouter := router.(*pathroute.PathRouter)

	l := pathRouter.GetLocationByPattern(location.Path)
	c.Assert(l, NotNil)
}

func makeLocation() (*Location, *Host) {
	host := &Host{Name: "localhost"}
	upstream := &Upstream{
		Id: "up1",
		Endpoints: []*Endpoint{
			{
				Url: "http://localhost:5000",
			},
		},
	}
	location := &Location{
		Hostname: host.Name,
		Path:     "/home",
		Id:       "loc1",
		Upstream: upstream,
	}
	return location, host
}
