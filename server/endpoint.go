package server

import (
	"fmt"
	"net/url"

	"github.com/BTBurke/vulcand/Godeps/_workspace/src/github.com/mailgun/vulcan/netutils"
	"github.com/BTBurke/vulcand/Godeps/_workspace/src/github.com/mailgun/vulcand/backend"
)

type muxEndpoint struct {
	url      *url.URL
	id       string
	location *backend.Location
	endpoint *backend.Endpoint
	mon      *perfMon
}

func newEndpoint(loc *backend.Location, e *backend.Endpoint, mon *perfMon) (*muxEndpoint, error) {
	url, err := netutils.ParseUrl(e.Url)
	if err != nil {
		return nil, err
	}
	return &muxEndpoint{location: loc, endpoint: e, id: e.GetUniqueId().String(), url: url, mon: mon}, nil
}

func (e *muxEndpoint) ResetStats() error {
	return e.mon.resetLocationStats(e.location)
}

func (e *muxEndpoint) GetStats() (*backend.RoundTripStats, error) {
	return e.mon.getLocationStats(e.location)
}

func (e *muxEndpoint) GetLocation() *backend.Location {
	return e.location
}

func (e *muxEndpoint) String() string {
	return fmt.Sprintf("muxEndpoint(id=%s, url=%s)", e.id, e.url.String())
}

func (e *muxEndpoint) GetId() string {
	return e.id
}

func (e *muxEndpoint) GetUrl() *url.URL {
	return e.url
}
