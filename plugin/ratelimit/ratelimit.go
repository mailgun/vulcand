package ratelimit

import (
	"fmt"
	"github.com/mailgun/vulcand/Godeps/_workspace/src/github.com/codegangsta/cli"
	"github.com/mailgun/vulcand/Godeps/_workspace/src/github.com/mailgun/vulcan/limit"
	"github.com/mailgun/vulcand/Godeps/_workspace/src/github.com/mailgun/vulcan/limit/tokenbucket"
	"github.com/mailgun/vulcand/Godeps/_workspace/src/github.com/mailgun/vulcan/middleware"
	"github.com/mailgun/vulcand/plugin"
	"time"
)

const Type = "ratelimit"

// Spec is an entry point of a plugin and will be called to register this middleware plugin withing vulcand
func GetSpec() *plugin.MiddlewareSpec {
	return &plugin.MiddlewareSpec{
		Type:      Type,
		FromOther: FromOther,
		FromCli:   FromCli,
		CliFlags:  CliFlags(),
	}
}

// Rate controls how many requests per period of time is allowed for a location.
// Existing implementation is based on the token bucket algorightm http://en.wikipedia.org/wiki/Token_bucket
type RateLimit struct {
	PeriodSeconds int    // Period in seconds, e.g. 3600 to set up hourly rates
	Burst         int64  // Burst count, allowes some extra variance for requests exceeding the average rate
	Variable      string // Variable defines how the limiting should be done. e.g. 'client.ip' or 'request.header.X-My-Header'
	Requests      int    // Allowed average requests
}

// Returns vulcan library compatible middleware
func (r *RateLimit) NewMiddleware() (middleware.Middleware, error) {
	mapper, err := limit.VariableToMapper(r.Variable)
	if err != nil {
		return nil, err
	}
	rate := tokenbucket.Rate{Units: int64(r.Requests), Period: time.Second * time.Duration(r.PeriodSeconds)}
	return tokenbucket.NewTokenLimiterWithOptions(mapper, rate, tokenbucket.Options{Burst: r.Burst})
}

func NewRateLimit(requests int, variable string, burst int64, periodSeconds int) (*RateLimit, error) {
	if _, err := limit.VariableToMapper(variable); err != nil {
		return nil, err
	}
	if requests <= 0 {
		return nil, fmt.Errorf("requests should be > 0, got %d", requests)
	}
	if burst < 0 {
		return nil, fmt.Errorf("burst should be >= 0, got %d", burst)
	}
	if periodSeconds <= 0 {
		return nil, fmt.Errorf("period seconds should be > 0, got %d", periodSeconds)
	}
	return &RateLimit{
		Requests:      requests,
		Variable:      variable,
		Burst:         burst,
		PeriodSeconds: periodSeconds,
	}, nil
}

func (rl *RateLimit) String() string {
	return fmt.Sprintf("var=%s, reqs/%s=%d, burst=%d",
		rl.Variable, time.Duration(rl.PeriodSeconds)*time.Second, rl.Requests, rl.Burst)
}

func FromOther(rate RateLimit) (plugin.MiddlewareFactory, error) {
	return NewRateLimit(rate.Requests, rate.Variable, rate.Burst, rate.PeriodSeconds)
}

// Constructs the middleware from the command line
func FromCli(c *cli.Context) (plugin.MiddlewareFactory, error) {
	return NewRateLimit(c.Int("requests"), c.String("var"), int64(c.Int("burst")), c.Int("period"))
}

func CliFlags() []cli.Flag {
	return []cli.Flag{
		cli.StringFlag{Name: "variable, var", Value: "client.ip", Usage: "variable to rate against, e.g. client.ip, request.host or request.header.X-Header"},
		cli.IntFlag{Name: "requests", Value: 1, Usage: "amount of requests"},
		cli.IntFlag{Name: "period", Value: 1, Usage: "rate limit period in seconds"},
		cli.IntFlag{Name: "burst", Value: 1, Usage: "allowed burst"},
	}
}
