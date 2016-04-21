package service

import (
	"flag"
	"fmt"
	log "github.com/Sirupsen/logrus"
	"github.com/coreos/etcd/pkg/flags"
	"os"
	"strings"
	"time"
)

type Options struct {
	*flag.FlagSet

	ApiPort      int
	ApiInterface string

	PidPath string
	Port    int

	Interface string
	CertPath  string

	EtcdNodes               listOptions
	EtcdKey                 string
	EtcdCaFile              string
	EtcdCertFile            string
	EtcdKeyFile             string
	EtcdConsistency         string
	EtcdSyncIntervalSeconds int64

	Log         string
	LogSeverity SeverityFlag

	ServerReadTimeout    time.Duration
	ServerWriteTimeout   time.Duration
	ServerMaxHeaderBytes int

	EndpointDialTimeout time.Duration
	EndpointReadTimeout time.Duration

	SealKey string

	StatsdAddr   string
	StatsdPrefix string

	DefaultListener bool
}

type SeverityFlag struct {
	S log.Level
}

func (s *SeverityFlag) Get() interface{} {
	return &s.S
}

// Set is part of the flag.Value interface.
func (s *SeverityFlag) Set(value string) error {
	sev, err := log.ParseLevel(strings.ToLower(value))
	if err != nil {
		return err
	}
	s.S = sev
	return nil
}

func (s *SeverityFlag) String() string {
	return s.S.String()
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
	})
	return o, nil
}

func ParseRunOptions() (options Options, err error) {
	options.FlagSet = flag.NewFlagSet("vulcand", flag.ContinueOnError)
	fs := options.FlagSet

	fs.Var(&options.EtcdNodes, "etcd", "Etcd discovery service API endpoints")
	fs.StringVar(&options.EtcdKey, "etcdKey", "vulcand", "Etcd key for storing configuration")
	fs.StringVar(&options.EtcdCaFile, "etcdCaFile", "", "Path to CA file for etcd communication")
	fs.StringVar(&options.EtcdCertFile, "etcdCertFile", "", "Path to cert file for etcd communication")
	fs.StringVar(&options.EtcdKeyFile, "etcdKeyFile", "", "Path to key file for etcd communication")
	fs.StringVar(&options.EtcdConsistency, "etcdConsistency", "STRONG", "Etcd consistency (STRONG or WEAK)")
	fs.Int64Var(&options.EtcdSyncIntervalSeconds, "etcdSyncIntervalSeconds", 0, "Interval between updating etcd cluster information. Use 0 to disable any syncing (default behavior.)")
	fs.StringVar(&options.PidPath, "pidPath", "", "Path to write PID file to")
	fs.IntVar(&options.Port, "port", 8181, "Port to listen on")
	fs.IntVar(&options.ApiPort, "apiPort", 8182, "Port to provide api on")

	fs.StringVar(&options.Interface, "interface", "", "Interface to bind to")
	fs.StringVar(&options.ApiInterface, "apiInterface", "", "Interface to for API to bind to")
	fs.StringVar(&options.CertPath, "certPath", "", "KeyPair to use (enables TLS)")
	fs.StringVar(&options.Log, "log", "console", "Logging to use (console, json, syslog or logstash)")

	options.LogSeverity.S = log.WarnLevel
	fs.Var(&options.LogSeverity, "logSeverity", "logs at or above this level to the logging output")

	fs.IntVar(&options.ServerMaxHeaderBytes, "serverMaxHeaderBytes", 1<<20, "Maximum size of request headers")
	fs.DurationVar(&options.ServerReadTimeout, "readTimeout", time.Duration(60)*time.Second, "HTTP server read timeout (deprecated)")
	fs.DurationVar(&options.ServerReadTimeout, "serverReadTimeout", time.Duration(60)*time.Second, "HTTP server read timeout")
	fs.DurationVar(&options.ServerWriteTimeout, "writeTimeout", time.Duration(60)*time.Second, "HTTP server write timeout (deprecated)")
	fs.DurationVar(&options.ServerWriteTimeout, "serverWriteTimeout", time.Duration(60)*time.Second, "HTTP server write timeout")
	fs.DurationVar(&options.EndpointDialTimeout, "endpointDialTimeout", time.Duration(5)*time.Second, "Endpoint dial timeout")
	fs.DurationVar(&options.EndpointReadTimeout, "endpointReadTimeout", time.Duration(50)*time.Second, "Endpoint read timeout")

	fs.StringVar(&options.SealKey, "sealKey", "", "Seal key used to store encrypted data in the backend")

	fs.StringVar(&options.StatsdPrefix, "statsdPrefix", "", "Statsd prefix will be appended to the metrics emitted by this instance")
	fs.StringVar(&options.StatsdAddr, "statsdAddr", "", "Statsd address in form of 'host:port'")

	fs.BoolVar(&options.DefaultListener, "default-listener", true, "Enables the default listener on startup (Default value: true)")

	options.FlagSet.Parse(os.Args[1:])
	err = flags.SetFlagsFromEnv("VULCAND", fs)
	if err != nil {
		fmt.Printf("Error passing env variables: %s\n", err)
	}
	options, err = validateOptions(options)
	if err != nil {
		return options, err
	}

	return options, nil
}
