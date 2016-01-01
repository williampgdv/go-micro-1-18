package cmd

import (
	"flag"
	"fmt"
	"io"
	"math/rand"
	"os"
	"strings"
	"time"

	"github.com/micro/cli"
	"github.com/micro/go-micro/broker"
	"github.com/micro/go-micro/client"
	"github.com/micro/go-micro/registry"
	"github.com/micro/go-micro/selector"
	"github.com/micro/go-micro/server"
	"github.com/micro/go-micro/transport"
)

type Cmd interface {
	// The cli app within this cmd
	App() *cli.App
	// Adds options, parses flags and initialise
	// exits on error
	Init(opts ...Option)
	// Options set within this command
	Options() Options
}

type cmd struct {
	opts Options
	app  *cli.App
}

type Option func(o *Options)

var (
	DefaultCmd = newCmd()

	DefaultFlags = []cli.Flag{
		cli.StringFlag{
			Name:   "server_name",
			EnvVar: "MICRO_SERVER_NAME",
			Usage:  "Name of the server. go.micro.srv.example",
		},
		cli.StringFlag{
			Name:   "server_version",
			EnvVar: "MICRO_SERVER_VERSION",
			Usage:  "Version of the server. 1.1.0",
		},
		cli.StringFlag{
			Name:   "server_id",
			EnvVar: "MICRO_SERVER_ID",
			Usage:  "Id of the server. Auto-generated if not specified",
		},
		cli.StringFlag{
			Name:   "server_address",
			EnvVar: "MICRO_SERVER_ADDRESS",
			Value:  ":0",
			Usage:  "Bind address for the server. 127.0.0.1:8080",
		},
		cli.StringFlag{
			Name:   "server_advertise",
			EnvVar: "MICRO_SERVER_ADVERTISE",
			Usage:  "Used instead of the server_address when registering with discovery. 127.0.0.1:8080",
		},
		cli.StringSliceFlag{
			Name:   "server_metadata",
			EnvVar: "MICRO_SERVER_METADATA",
			Value:  &cli.StringSlice{},
			Usage:  "A list of key-value pairs defining metadata. version=1.0.0",
		},
		cli.StringFlag{
			Name:   "broker",
			EnvVar: "MICRO_BROKER",
			Value:  "http",
			Usage:  "Broker for pub/sub. http, nats, rabbitmq",
		},
		cli.StringFlag{
			Name:   "broker_address",
			EnvVar: "MICRO_BROKER_ADDRESS",
			Usage:  "Comma-separated list of broker addresses",
		},
		cli.StringFlag{
			Name:   "registry",
			EnvVar: "MICRO_REGISTRY",
			Value:  "consul",
			Usage:  "Registry for discovery. memory, consul, etcd, kubernetes",
		},
		cli.StringFlag{
			Name:   "registry_address",
			EnvVar: "MICRO_REGISTRY_ADDRESS",
			Usage:  "Comma-separated list of registry addresses",
		},
		cli.StringFlag{
			Name:   "selector",
			EnvVar: "MICRO_SELECTOR",
			Value:  "selector",
			Usage:  "Selector used to pick nodes for querying. random, roundrobin, blacklist",
		},
		cli.StringFlag{
			Name:   "transport",
			EnvVar: "MICRO_TRANSPORT",
			Value:  "http",
			Usage:  "Transport mechanism used; http, rabbitmq, nats",
		},
		cli.StringFlag{
			Name:   "transport_address",
			EnvVar: "MICRO_TRANSPORT_ADDRESS",
			Usage:  "Comma-separated list of transport addresses",
		},

		// logging flags
		cli.BoolFlag{
			Name:  "logtostderr",
			Usage: "log to standard error instead of files",
		},
		cli.BoolFlag{
			Name:  "alsologtostderr",
			Usage: "log to standard error as well as files",
		},
		cli.StringFlag{
			Name:  "log_dir",
			Usage: "log files will be written to this directory instead of the default temporary directory",
		},
		cli.StringFlag{
			Name:  "stderrthreshold",
			Usage: "logs at or above this threshold go to stderr",
		},
		cli.StringFlag{
			Name:  "v",
			Usage: "log level for V logs",
		},
		cli.StringFlag{
			Name:  "vmodule",
			Usage: "comma-separated list of pattern=N settings for file-filtered logging",
		},
		cli.StringFlag{
			Name:  "log_backtrace_at",
			Usage: "when logging hits line file:N, emit a stack trace",
		},
	}

	DefaultBrokers = map[string]func([]string, ...broker.Option) broker.Broker{
		"http": broker.NewBroker,
	}

	DefaultRegistries = map[string]func([]string, ...registry.Option) registry.Registry{
		"consul": registry.NewRegistry,
	}

	DefaultSelectors = map[string]func(...selector.Option) selector.Selector{
		"random": selector.NewSelector,
	}

	DefaultTransports = map[string]func([]string, ...transport.Option) transport.Transport{
		"http": transport.NewTransport,
	}
)

func init() {
	rand.Seed(time.Now().Unix())
	help := cli.HelpPrinter
	cli.HelpPrinter = func(writer io.Writer, templ string, data interface{}) {
		help(writer, templ, data)
		os.Exit(0)
	}
}

func newCmd(opts ...Option) Cmd {
	options := Options{
		Brokers:    DefaultBrokers,
		Registries: DefaultRegistries,
		Selectors:  DefaultSelectors,
		Transports: DefaultTransports,
	}

	for _, o := range opts {
		o(&options)
	}

	if len(options.Description) == 0 {
		options.Description = "a go-micro service"
	}

	cmd := new(cmd)
	cmd.opts = options
	cmd.app = cli.NewApp()
	cmd.app.Name = cmd.opts.Name
	cmd.app.Version = cmd.opts.Version
	cmd.app.Usage = cmd.opts.Description
	cmd.app.Before = cmd.Before
	cmd.app.Flags = DefaultFlags
	cmd.app.Action = func(c *cli.Context) {}

	if len(options.Version) == 0 {
		cmd.app.HideVersion = true
	}

	return cmd
}

func (c *cmd) App() *cli.App {
	return c.app
}

func (c *cmd) Options() Options {
	return c.opts
}

func (c *cmd) Before(ctx *cli.Context) error {
	// Due to logger issues with glog, we need to do this
	os.Args = os.Args[:1]
	flag.Set("logtostderr", fmt.Sprintf("%v", ctx.Bool("logtostderr")))
	flag.Set("alsologtostderr", fmt.Sprintf("%v", ctx.Bool("alsologtostderr")))
	flag.Set("stderrthreshold", ctx.String("stderrthreshold"))
	flag.Set("log_backtrace_at", ctx.String("log_backtrace_at"))
	flag.Set("log_dir", ctx.String("log_dir"))
	flag.Set("vmodule", ctx.String("vmodule"))
	flag.Set("v", ctx.String("v"))
	flag.Parse()

	if b, ok := c.opts.Brokers[ctx.String("broker")]; ok {
		broker.DefaultBroker = b(strings.Split(ctx.String("broker_address"), ","))
	}

	if r, ok := c.opts.Registries[ctx.String("registry")]; ok {
		registry.DefaultRegistry = r(strings.Split(ctx.String("registry_address"), ","))
	}

	if s, ok := c.opts.Selectors[ctx.String("selector")]; ok {
		selector.DefaultSelector = s(selector.Registry(registry.DefaultRegistry))
	}

	if t, ok := c.opts.Transports[ctx.String("transport")]; ok {
		transport.DefaultTransport = t(strings.Split(ctx.String("transport_address"), ","))
	}

	metadata := make(map[string]string)
	for _, d := range ctx.StringSlice("server_metadata") {
		var key, val string
		parts := strings.Split(d, "=")
		key = parts[0]
		if len(parts) > 1 {
			val = strings.Join(parts[1:], "=")
		}
		metadata[key] = val
	}

	server.DefaultServer = server.NewServer(
		server.Name(ctx.String("server_name")),
		server.Version(ctx.String("server_version")),
		server.Id(ctx.String("server_id")),
		server.Address(ctx.String("server_address")),
		server.Advertise(ctx.String("server_advertise")),
		server.Metadata(metadata),
	)

	client.DefaultClient = client.NewClient()

	return nil
}

func (c *cmd) Init(opts ...Option) {
	for _, o := range opts {
		o(&c.opts)
	}
	c.app.Name = c.opts.Name
	c.app.Version = c.opts.Version
	c.app.Usage = c.opts.Description
	c.app.RunAndExitOnError()
}

func Init(opts ...Option) {
	DefaultCmd.Init(opts...)
}

func NewCmd(opts ...Option) Cmd {
	return newCmd(opts...)
}
