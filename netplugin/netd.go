/***
Copyright 2014 Cisco Systems Inc. All rights reserved.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"flag"
	"fmt"
	"log/syslog"
	"net/url"
	"os"
	"os/user"
	"strings"
	"time"

	"github.com/contiv/netplugin/core"
	"github.com/contiv/netplugin/netplugin/agent"
	"github.com/contiv/netplugin/netplugin/cluster"
	"github.com/contiv/netplugin/netplugin/plugin"
	"github.com/contiv/netplugin/version"

	log "github.com/Sirupsen/logrus"
	"github.com/Sirupsen/logrus/hooks/syslog"
)

// StringSlice is a flag option implementation
type StringSlice []string

func (s *StringSlice) String() string {
	return fmt.Sprintf("%s", *s)
}

// Get returns the slice of strings set by this flag
func (s *StringSlice) Get() interface{} {
	return *s
}

// Set function for StringSlice sets the appropriate field for flag handling
func (s *StringSlice) Set(value string) error {
	optVal := strings.Split(value, ",")
	*s = append(*s, optVal...)
	return nil
}

// Value returns the slice of strings set by this flag
func (s *StringSlice) Value() []string {
	return *s
}

// a daemon based on etcd client's Watch interface to trigger plugin's
// network provisioning interfaces

type cliOpts struct {
	hostLabel  string
	pluginMode string // plugin could be docker | kubernetes
	cfgFile    string
	debug      bool
	syslog     string
	jsonLog    bool
	ctrlIP     string      // IP address to be used by control protocols
	vtepIP     string      // IP address to be used by the VTEP
	vlanIntf   StringSlice // Uplink interface for VLAN switching
	version    bool
	dbURL      string // state store URL
}

func configureSyslog(syslogParam string) {
	var err error
	var hook log.Hook

	// disable colors if we're writing to syslog *and* we're the default text
	// formatter, because the tty detection is useless here.
	if tf, ok := log.StandardLogger().Formatter.(*log.TextFormatter); ok {
		tf.DisableColors = true
	}

	if syslogParam == "kernel" {
		hook, err = logrus_syslog.NewSyslogHook("", "", syslog.LOG_INFO, "netplugin")
		if err != nil {
			log.Fatalf("Could not connect to kernel syslog")
		}
	} else {
		u, err := url.Parse(syslogParam)
		if err != nil {
			log.Fatalf("Could not parse syslog spec: %v", err)
		}

		hook, err = logrus_syslog.NewSyslogHook(u.Scheme, u.Host, syslog.LOG_INFO, "netplugin")
		if err != nil {
			log.Fatalf("Could not connect to syslog: %v", err)
		}
	}

	log.AddHook(hook)
}

func main() {
	var opts cliOpts
	var flagSet *flag.FlagSet

	defHostLabel, err := os.Hostname()

	// parse rest of the args that require creating state
	flagSet = flag.NewFlagSet("netplugin", flag.ExitOnError)
	flagSet.BoolVar(&opts.debug,
		"debug",
		false,
		"Show debugging information generated by netplugin")
	flagSet.StringVar(&opts.syslog,
		"syslog",
		"",
		"Log to syslog at proto://ip:port -- use 'kernel' to log via kernel syslog")
	flagSet.BoolVar(&opts.jsonLog,
		"json-log",
		false,
		"Format logs as JSON")
	flagSet.StringVar(&opts.hostLabel,
		"host-label",
		defHostLabel,
		"label used to identify endpoints homed for this host, default is host name. If -config flag is used then host-label must be specified in the the configuration passed.")
	flagSet.StringVar(&opts.pluginMode,
		"plugin-mode",
		"docker",
		"plugin mode docker|kubernetes")
	flagSet.StringVar(&opts.cfgFile,
		"config",
		"",
		"plugin configuration. Use '-' to read configuration from stdin")
	flagSet.StringVar(&opts.vtepIP,
		"vtep-ip",
		"",
		"My VTEP ip address")
	flagSet.StringVar(&opts.ctrlIP,
		"ctrl-ip",
		"",
		"Local ip address to be used for control communication")
	flagSet.Var(&opts.vlanIntf,
		"vlan-if",
		"VLAN uplink interface")
	flagSet.BoolVar(&opts.version,
		"version",
		false,
		"Show version")
	flagSet.StringVar(&opts.dbURL,
		"cluster-store",
		"etcd://127.0.0.1:2379",
		"state store url")

	err = flagSet.Parse(os.Args[1:])
	if err != nil {
		log.Fatalf("Failed to parse command. Error: %s", err)
	}

	if opts.version {
		fmt.Printf(version.String())
		os.Exit(0)
	}

	// Make sure we are running as root
	usr, err := user.Current()
	if (err != nil) || (usr.Username != "root") {
		log.Fatalf("This process can only be run as root")
	}

	if opts.debug {
		log.SetLevel(log.DebugLevel)
		os.Setenv("CONTIV_TRACE", "1")
	}

	if opts.jsonLog {
		log.SetFormatter(&log.JSONFormatter{})
	} else {
		log.SetFormatter(&log.TextFormatter{FullTimestamp: true, TimestampFormat: time.StampNano})
	}

	if opts.syslog != "" {
		configureSyslog(opts.syslog)
	}

	if flagSet.NFlag() < 1 {
		log.Infof("host-label not specified, using default (%s)", opts.hostLabel)
	}

	// default to using local IP addr
	localIP, err := cluster.GetLocalAddr()
	if err != nil {
		log.Fatalf("Error getting local address. Err: %v", err)
	}
	if opts.ctrlIP == "" {
		opts.ctrlIP = localIP
	}
	if opts.vtepIP == "" {
		opts.vtepIP = opts.ctrlIP
	}

	// parse store URL
	parts := strings.Split(opts.dbURL, "://")
	if len(parts) < 2 {
		log.Fatalf("Invalid cluster-store-url %s", opts.dbURL)
	}
	stateStore := parts[0]

	// initialize the config
	pluginConfig := plugin.Config{
		Drivers: plugin.Drivers{
			Network: "ovs",
			State:   stateStore,
		},
		Instance: core.InstanceInfo{
			HostLabel:  opts.hostLabel,
			CtrlIP:     opts.ctrlIP,
			VtepIP:     opts.vtepIP,
			UplinkIntf: opts.vlanIntf,
			DbURL:      opts.dbURL,
			PluginMode: opts.pluginMode,
		},
	}

	// Create a new agent
	ag := agent.NewAgent(&pluginConfig)

	// Process all current state
	ag.ProcessCurrentState()

	// post initialization processing
	ag.PostInit()

	// handle events
	if err := ag.HandleEvents(); err != nil {
		log.Infof("Netplugin exiting due to error: %v", err)
		os.Exit(1)
	}
}
