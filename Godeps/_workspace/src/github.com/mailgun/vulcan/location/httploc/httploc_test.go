package httploc

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	timetools "github.com/mailgun/vulcand/Godeps/_workspace/src/github.com/mailgun/gotools-time"
	"github.com/mailgun/vulcand/Godeps/_workspace/src/github.com/mailgun/vulcan"
	. "github.com/mailgun/vulcand/Godeps/_workspace/src/github.com/mailgun/vulcan/endpoint"
	"github.com/mailgun/vulcand/Godeps/_workspace/src/github.com/mailgun/vulcan/headers"
	. "github.com/mailgun/vulcand/Godeps/_workspace/src/github.com/mailgun/vulcan/loadbalance"
	"github.com/mailgun/vulcand/Godeps/_workspace/src/github.com/mailgun/vulcan/loadbalance/roundrobin"
	. "github.com/mailgun/vulcand/Godeps/_workspace/src/github.com/mailgun/vulcan/middleware"
	"github.com/mailgun/vulcand/Godeps/_workspace/src/github.com/mailgun/vulcan/netutils"
	. "github.com/mailgun/vulcand/Godeps/_workspace/src/github.com/mailgun/vulcan/request"
	. "github.com/mailgun/vulcand/Godeps/_workspace/src/github.com/mailgun/vulcan/route"
	. "github.com/mailgun/vulcand/Godeps/_workspace/src/github.com/mailgun/vulcan/testutils"
	. "github.com/mailgun/vulcand/Godeps/_workspace/src/gopkg.in/check.v1"
)

type LocSuite struct {
	authHeaders http.Header
	tm          *timetools.FreezedTime
}

func Test(t *testing.T) { TestingT(t) }

var _ = Suite(&LocSuite{
	authHeaders: http.Header{
		"Authorization": []string{"Basic QWxhZGRpbjpvcGVuIHNlc2FtZQ=="},
	},
	tm: &timetools.FreezedTime{
		CurrentTime: time.Date(2012, 3, 4, 5, 6, 7, 0, time.UTC),
	},
})

func (s *LocSuite) newRoundRobin(endpoints ...string) LoadBalancer {
	rr, err := roundrobin.NewRoundRobinWithOptions(roundrobin.Options{TimeProvider: s.tm})
	if err != nil {
		panic(err)
	}
	for _, e := range endpoints {
		rr.AddEndpoint(MustParseUrl(e))
	}
	return rr
}

func (s *LocSuite) newProxyWithParams(
	l LoadBalancer,
	readTimeout time.Duration,
	dialTimeout time.Duration,
	maxMemBytes int64,
	maxBodyBytes int64) (*HttpLocation, *httptest.Server) {

	location, err := NewLocationWithOptions("dummy", l, Options{
		TrustForwardHeader: true,
		Limits: Limits{
			MaxMemBodyBytes: maxMemBytes,
			MaxBodyBytes:    maxBodyBytes,
		},
	})
	if err != nil {
		panic(err)
	}
	proxy, err := vulcan.NewProxy(&ConstRouter{
		Location: location,
	})
	if err != nil {
		panic(err)
	}
	return location, httptest.NewServer(proxy)
}

func (s *LocSuite) newProxy(l LoadBalancer) (*HttpLocation, *httptest.Server) {
	return s.newProxyWithParams(l, time.Duration(0), time.Duration(0), int64(0), int64(0))
}

// No avialable endpoints
func (s *LocSuite) TestNoEndpoints(c *C) {
	_, proxy := s.newProxy(s.newRoundRobin())
	defer proxy.Close()

	response, _ := Get(c, proxy.URL, s.authHeaders, "hello!")
	c.Assert(response.StatusCode, Equals, http.StatusBadGateway)
}

// No avialable endpoints
func (s *LocSuite) TestAllEndpointsAreDown(c *C) {
	_, proxy := s.newProxy(s.newRoundRobin("http://localhost:63999"))
	defer proxy.Close()

	response, _ := Get(c, proxy.URL, s.authHeaders, "hello!")
	c.Assert(response.StatusCode, Equals, http.StatusBadGateway)
}

// Success, make sure we've successfully proxied the response
func (s *LocSuite) TestSuccess(c *C) {
	server := NewTestServer(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("Hi, I'm endpoint"))
	})
	defer server.Close()

	_, proxy := s.newProxy(s.newRoundRobin(server.URL))
	defer proxy.Close()

	response, bodyBytes := Get(c, proxy.URL, s.authHeaders, "hello!")
	c.Assert(response.StatusCode, Equals, http.StatusOK)
	c.Assert(string(bodyBytes), Equals, "Hi, I'm endpoint")
}

// Success, make sure we've successfully proxied the response when limit was set but not reached
func (s *LocSuite) TestSuccessLimitNotReached(c *C) {
	server := NewTestServer(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("Hi, I'm endpoint"))
	})
	defer server.Close()

	_, proxy := s.newProxyWithParams(s.newRoundRobin(server.URL), 0, 0, 4, 4096)
	defer proxy.Close()

	response, bodyBytes := Get(c, proxy.URL, s.authHeaders, "hello!")
	c.Assert(response.StatusCode, Equals, http.StatusOK)
	c.Assert(string(bodyBytes), Equals, "Hi, I'm endpoint")
}

func (s *LocSuite) TestChunkedEncodingSuccess(c *C) {
	requestBody := ""
	contentLength := int64(0)
	server := NewTestServer(func(w http.ResponseWriter, r *http.Request) {
		body, err := ioutil.ReadAll(r.Body)
		c.Assert(err, IsNil)
		requestBody = string(body)
		contentLength = r.ContentLength
		w.Write([]byte("Hi, I'm endpoint"))
	})
	defer server.Close()

	_, proxy := s.newProxyWithParams(s.newRoundRobin(server.URL), 0, 0, 4, 4096)
	defer proxy.Close()

	conn, err := net.Dial("tcp", netutils.MustParseUrl(proxy.URL).Host)
	c.Assert(err, IsNil)
	fmt.Fprintf(conn, "POST / HTTP/1.0\r\nTransfer-Encoding: chunked\r\n\r\n4\r\ntest\r\n5\r\ntest1\r\n5\r\ntest2\r\n0\r\n\r\n")
	status, err := bufio.NewReader(conn).ReadString('\n')

	c.Assert(requestBody, Equals, "testtest1test2")
	c.Assert(status, Equals, "HTTP/1.0 200 OK\r\n")
	c.Assert(contentLength, Equals, int64(len(requestBody)))
}

func (s *LocSuite) TestLimitReached(c *C) {
	server := NewTestServer(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("Hi, I'm endpoint!"))
	})
	defer server.Close()

	_, proxy := s.newProxyWithParams(s.newRoundRobin(server.URL), 0, 0, 4, 8)
	defer proxy.Close()

	response, _ := Get(c, proxy.URL, s.authHeaders, "Hello, this request is longer than 8 bytes")
	c.Assert(response.StatusCode, Equals, http.StatusRequestEntityTooLarge)
}

func (s *LocSuite) TestChunkedEncodingLimitReached(c *C) {
	server := NewTestServer(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("Hi, I'm endpoint"))
	})
	defer server.Close()

	_, proxy := s.newProxyWithParams(s.newRoundRobin(server.URL), 0, 0, 4, 8)
	defer proxy.Close()

	conn, err := net.Dial("tcp", netutils.MustParseUrl(proxy.URL).Host)
	c.Assert(err, IsNil)
	fmt.Fprintf(conn, "POST / HTTP/1.0\r\nTransfer-Encoding: chunked\r\n\r\n4\r\ntest\r\n5\r\ntest1\r\n5\r\ntest2\r\n0\r\n\r\n")
	status, err := bufio.NewReader(conn).ReadString('\n')

	c.Assert(status, Equals, "HTTP/1.0 413 Request Entity Too Large\r\n")
}

func (s *LocSuite) TestUpdateLimit(c *C) {
	server := NewTestServer(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("Hi, I'm endpoint!"))
	})
	defer server.Close()

	location, proxy := s.newProxyWithParams(s.newRoundRobin(server.URL), 0, 0, 4, 1024)
	defer proxy.Close()

	response, _ := Get(c, proxy.URL, s.authHeaders, "Hello, this request is longer than 8 bytes")
	c.Assert(response.StatusCode, Equals, http.StatusOK)

	options := location.GetOptions()
	options.Limits.MaxBodyBytes = 8
	err := location.SetOptions(options)
	c.Assert(err, IsNil)

	response, _ = Get(c, proxy.URL, s.authHeaders, "Hello, this request is longer than 8 bytes")
	c.Assert(response.StatusCode, Equals, http.StatusRequestEntityTooLarge)
}

func (s *LocSuite) TestUpdateForwardHeader(c *C) {
	var header string
	server := NewTestServer(func(w http.ResponseWriter, r *http.Request) {
		header = r.Header.Get("X-Forwarded-Server")
		w.Write([]byte("Hi, I'm endpoint!"))
	})
	defer server.Close()

	location, proxy := s.newProxy(s.newRoundRobin(server.URL))
	defer proxy.Close()

	options := location.GetOptions()
	options.Hostname = "host1"
	location.SetOptions(options)

	response, _ := Get(c, proxy.URL, s.authHeaders, "Hello")
	c.Assert(response.StatusCode, Equals, http.StatusOK)
	c.Assert(header, Equals, "host1")

	options = location.GetOptions()
	options.Hostname = "host2"
	err := location.SetOptions(options)
	c.Assert(err, IsNil)

	response, _ = Get(c, proxy.URL, s.authHeaders, "Hello")
	c.Assert(response.StatusCode, Equals, http.StatusOK)
	c.Assert(header, Equals, "host2")
}

func (s *LocSuite) TestFailover(c *C) {
	server := NewTestServer(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("Hi, I'm endpoint"))
	})
	defer server.Close()

	_, proxy := s.newProxy(s.newRoundRobin("http://localhost:63999", server.URL))
	defer proxy.Close()

	response, bodyBytes := Get(c, proxy.URL, s.authHeaders, "hello!")
	c.Assert(response.StatusCode, Equals, http.StatusOK)
	c.Assert(string(bodyBytes), Equals, "Hi, I'm endpoint")
}

// Test scenario when middleware intercepts the request
func (s *LocSuite) TestMiddlewareInterceptsRequest(c *C) {
	server := NewTestServer(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("Hi, I'm endpoint"))
	})
	defer server.Close()

	location, proxy := s.newProxy(s.newRoundRobin(server.URL))
	defer proxy.Close()

	calls := make(map[string]int)

	auth := &MiddlewareWrapper{
		OnRequest: func(r Request) (*http.Response, error) {
			calls["authReq"] += 1
			return netutils.NewTextResponse(
				r.GetHttpRequest(),
				http.StatusForbidden,
				"Intercepted Request"), nil
		},
		OnResponse: func(r Request, a Attempt) {
			calls["authRe"] += 1
		},
	}

	location.GetMiddlewareChain().Add("auth", 0, auth)

	response, bodyBytes := Get(c, proxy.URL, s.authHeaders, "hello!")
	c.Assert(response.StatusCode, Equals, http.StatusForbidden)
	c.Assert(string(bodyBytes), Equals, "Intercepted Request")

	// Auth middleware has been called on response as well
	c.Assert(calls["authReq"], Equals, 1)
	c.Assert(calls["authRe"], Equals, 1)
}

// Test scenario when middleware intercepts the request
func (s *LocSuite) TestMultipleMiddlewaresRequestIntercepted(c *C) {
	server := NewTestServer(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("Hi, I'm endpoint"))
	})
	defer server.Close()

	location, proxy := s.newProxy(s.newRoundRobin(server.URL))
	defer proxy.Close()

	calls := make(map[string]int)

	auth := &MiddlewareWrapper{
		OnRequest: func(r Request) (*http.Response, error) {
			calls["authReq"] += 1
			return netutils.NewTextResponse(
				r.GetHttpRequest(),
				http.StatusForbidden,
				"Intercepted Request"), nil
		},
		OnResponse: func(r Request, a Attempt) {
			calls["authRe"] += 1
		},
	}

	cb := &MiddlewareWrapper{
		OnRequest: func(r Request) (*http.Response, error) {
			calls["cbReq"] += 1
			return nil, nil
		},
		OnResponse: func(r Request, a Attempt) {
			calls["cbRe"] += 1
		},
	}

	observer := &ObserverWrapper{
		OnRequest: func(r Request) {
			calls["oReq"] += 1
		},
		OnResponse: func(r Request, a Attempt) {
			calls["oRe"] += 1
		},
	}

	location.GetMiddlewareChain().Add("auth", 0, auth)
	location.GetMiddlewareChain().Add("cb", 1, cb)
	location.GetObserverChain().Add("ob", observer)

	response, bodyBytes := Get(c, proxy.URL, s.authHeaders, "hello!")
	c.Assert(response.StatusCode, Equals, http.StatusForbidden)
	c.Assert(string(bodyBytes), Equals, "Intercepted Request")

	// Auth middleware has been called on response as well
	c.Assert(calls["authReq"], Equals, 1)
	c.Assert(calls["authRe"], Equals, 1)

	// Callback has never got to a request, because it was intercepted
	c.Assert(calls["cbReq"], Equals, 0)
	c.Assert(calls["cbRe"], Equals, 0)

	// Observer was called regardless
	c.Assert(calls["oReq"], Equals, 1)
	c.Assert(calls["oRe"], Equals, 1)
}

// Test that X-Forwarded-For and X-Forwarded-Proto are passed through
func (s *LocSuite) TestForwardedHeaders(c *C) {
	server := NewTestServer(func(w http.ResponseWriter, r *http.Request) {
		c.Assert(r.Header.Get(headers.XForwardedProto), Equals, "httpx")
		c.Assert(r.Header.Get(headers.XForwardedFor), Equals, "192.168.1.1, 127.0.0.1")
	})
	defer server.Close()

	_, proxy := s.newProxy(s.newRoundRobin(server.URL))
	defer proxy.Close()

	hdr := http.Header(make(map[string][]string))
	hdr.Set(headers.XForwardedProto, "httpx")
	hdr.Set(headers.XForwardedFor, "192.168.1.1")

	Get(c, proxy.URL, hdr, "hello!")
}

// Test scenario when middleware intercepts the request
func (s *LocSuite) TestMiddlewareAddsHeader(c *C) {
	var capturedHeader []string
	server := NewTestServer(func(w http.ResponseWriter, r *http.Request) {
		capturedHeader = r.Header["X-Vulcan-Call"]
		w.Write([]byte("Hi, I'm endpoint"))
	})
	defer server.Close()

	location, proxy := s.newProxy(s.newRoundRobin(server.URL))
	defer proxy.Close()

	m := &MiddlewareWrapper{
		OnRequest: func(r Request) (*http.Response, error) {
			r.GetHttpRequest().Header["X-Vulcan-Call"] = []string{"hello"}
			return nil, nil
		},
		OnResponse: func(r Request, a Attempt) {
		},
	}

	location.GetMiddlewareChain().Add("m", 0, m)

	response, bodyBytes := Get(c, proxy.URL, nil, "hello!")
	c.Assert(response.StatusCode, Equals, http.StatusOK)
	c.Assert(capturedHeader, DeepEquals, []string{"hello"})
	c.Assert(string(bodyBytes), Equals, "Hi, I'm endpoint")
}

// Make sure that request gets cleaned up in case of the failover
// and the changes made by middlewares are being erased
func (s *LocSuite) TestMiddlewareAddsHeaderOnFailover(c *C) {

	var capturedHeader []string
	server := NewTestServer(func(w http.ResponseWriter, r *http.Request) {
		capturedHeader = r.Header["X-Vulcan-Call"]
		w.Write([]byte("Hi, I'm endpoint"))
	})
	defer server.Close()

	location, proxy := s.newProxy(s.newRoundRobin("http://localhost:63999", server.URL))
	defer proxy.Close()

	counter := 0
	m := &MiddlewareWrapper{
		OnRequest: func(r Request) (*http.Response, error) {
			r.GetHttpRequest().Header["X-Vulcan-Call"] = []string{fmt.Sprintf("hello %d", counter)}
			counter += 1
			return nil, nil
		},
		OnResponse: func(r Request, a Attempt) {
		},
	}

	location.GetMiddlewareChain().Add("m", 0, m)

	response, bodyBytes := Get(c, proxy.URL, nil, "hello!")
	c.Assert(response.StatusCode, Equals, http.StatusOK)
	c.Assert(capturedHeader, DeepEquals, []string{"hello 1"})
	c.Assert(string(bodyBytes), Equals, "Hi, I'm endpoint")
}

// Make sure that middleware changes do not propagate during failover
func (s *LocSuite) TestFailoverHeaders(c *C) {
	var finalHeaders []string
	server := NewTestServer(func(w http.ResponseWriter, r *http.Request) {
		finalHeaders = r.Header["X-Vulcan-Call"]
		w.Write([]byte("Hi, I'm endpoint"))
	})
	defer server.Close()

	location, proxy := s.newProxy(s.newRoundRobin("http://localhost:63999", server.URL))
	defer proxy.Close()

	// This middleware will be executed on both attempts.
	// We need to make sure that the first attempt does not interfere with the other.
	m := &MiddlewareWrapper{
		OnRequest: func(r Request) (*http.Response, error) {
			r.GetHttpRequest().Header.Add("X-Vulcan-Call", "call")
			return nil, nil
		},
		OnResponse: func(r Request, a Attempt) {
		},
	}
	location.GetMiddlewareChain().Add("m", 0, m)

	response, _ := Get(c, proxy.URL, s.authHeaders, "hello!")
	c.Assert(response.StatusCode, Equals, http.StatusOK)
	c.Assert(finalHeaders, DeepEquals, []string{"call"})
}

func (s *LocSuite) TestRewritesURLsWithEncodedPath(c *C) {
	var actualURL string

	server := NewTestServer(func(w http.ResponseWriter, r *http.Request) {
		actualURL = r.RequestURI
	})
	defer server.Close()

	_, proxy := s.newProxy(s.newRoundRobin(server.URL))
	defer proxy.Close()

	path := "/log/http%3A%2F%2Fwww.site.com%2Fsomething"
	url := netutils.MustParseUrl(proxy.URL)
	url.Opaque = path

	request, err := http.NewRequest("GET", url.String(), nil)
	request.URL = url

	http.DefaultClient.Do(request)

	c.Assert(err, IsNil)
	c.Assert(actualURL, Equals, path)
}
