package service

import (
	"flag"
	"fmt"
	"github.com/mailgun/vulcand/Godeps/_workspace/src/github.com/mailgun/go-etcd/etcd"
	"time"
)

type Options struct {
	ApiPort      int
	ApiInterface string
	PidPath      string
	Port         int
	Interface    string
	CertPath     string

	EtcdNodes       listOptions
	EtcdKey         string
	EtcdConsistency string
	Log             string

	ServerReadTimeout    time.Duration
	ServerWriteTimeout   time.Duration
	ServerMaxHeaderBytes int

	EndpointDialTimeout time.Duration
	EndpointReadTimeout time.Duration

	SealKey string
}

// Helper to parse options that can occur several times, e.g. cassandra nodes
type listOptions []string

func (o *listOptions) String() string {
	return fmt.Sprint(*o)
}

func (o *listOptions) Set(value string) error {
	*o = append(*o, value)
	return nil
}

func validateOptions(o Options) (Options, error) {
	if o.EndpointDialTimeout+o.EndpointReadTimeout >= o.ServerWriteTimeout {
		fmt.Printf("!!!!!! WARN: serverWriteTimout(%s) should be > endpointDialTimeout(%s) + endpointReadTimeout(%s)\n\n",
			o.ServerWriteTimeout, o.EndpointDialTimeout, o.EndpointReadTimeout)
	}
	flag.Visit(func(f *flag.Flag) {
		if f.Name == "readTimeout" {
			fmt.Printf("!!!!!! WARN: Using deprecated readTimeout flag, use serverReadTimeout instead\n\n")
		}
		if f.Name == "writeTimeout" {
			fmt.Printf("!!!!!! WARN: Using deprecated writeTimeout flag, use serverWriteTimeout instead\n\n")
		}
		if f.Name == "port" {
			fmt.Printf("!!!!!! WARN: Using deprecated port flag, use Listeners instead\n\n")
		}
		if f.Name == "interface" {
			fmt.Printf("!!!!!! WARN: Using deprecated interface flag, use Listeners instead\n\n")
		}
	})
	return o, nil
}

func ParseCommandLine() (options Options, err error) {
	flag.Var(&options.EtcdNodes, "etcd", "Etcd discovery service API endpoints")
	flag.StringVar(&options.EtcdKey, "etcdKey", "vulcand", "Etcd key for storing configuration")
	flag.StringVar(&options.EtcdConsistency, "etcdConsistency", etcd.STRONG_CONSISTENCY, "Etcd consistency")
	flag.StringVar(&options.PidPath, "pidPath", "", "Path to write PID file to")
	flag.IntVar(&options.Port, "port", 8181, "Port to listen on")
	flag.IntVar(&options.ApiPort, "apiPort", 8182, "Port to provide api on")

	flag.StringVar(&options.Interface, "interface", "", "Interface to bind to")
	flag.StringVar(&options.ApiInterface, "apiInterface", "", "Interface to for API to bind to")
	flag.StringVar(&options.CertPath, "certPath", "", "Certificate to use (enables TLS)")
	flag.StringVar(&options.Log, "log", "console", "Logging to use (syslog or console)")

	flag.IntVar(&options.ServerMaxHeaderBytes, "serverMaxHeaderBytes", 1<<20, "Maximum size of request headers")
	flag.DurationVar(&options.ServerReadTimeout, "readTimeout", time.Duration(60)*time.Second, "HTTP server read timeout (deprecated)")
	flag.DurationVar(&options.ServerReadTimeout, "serverReadTimeout", time.Duration(60)*time.Second, "HTTP server read timeout")
	flag.DurationVar(&options.ServerWriteTimeout, "writeTimeout", time.Duration(60)*time.Second, "HTTP server write timeout (deprecated)")
	flag.DurationVar(&options.ServerWriteTimeout, "serverWriteTimeout", time.Duration(60)*time.Second, "HTTP server write timeout")
	flag.DurationVar(&options.EndpointDialTimeout, "endpointDialTimeout", time.Duration(5)*time.Second, "Endpoint dial timeout")
	flag.DurationVar(&options.EndpointReadTimeout, "endpointReadTimeout", time.Duration(50)*time.Second, "Endpoint read timeout")

	flag.StringVar(&options.SealKey, "sealKey", "", "Seal key used to store encrypted data in the backend")

	flag.Parse()
	options, err = validateOptions(options)
	if err != nil {
		return options, err
	}
	return options, nil
}
