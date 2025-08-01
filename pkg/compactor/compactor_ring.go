package compactor

import (
	"flag"
	"os"
	"time"

	"github.com/go-kit/log/level"

	"github.com/cortexproject/cortex/pkg/ring"
	"github.com/cortexproject/cortex/pkg/ring/kv"
	"github.com/cortexproject/cortex/pkg/util/flagext"
	util_log "github.com/cortexproject/cortex/pkg/util/log"
)

// RingConfig masks the ring lifecycler config which contains
// many options not really required by the compactors ring. This config
// is used to strip down the config to the minimum, and avoid confusion
// to the user.
type RingConfig struct {
	KVStore                kv.Config     `yaml:"kvstore"`
	HeartbeatPeriod        time.Duration `yaml:"heartbeat_period"`
	HeartbeatTimeout       time.Duration `yaml:"heartbeat_timeout"`
	AutoForgetDelay        time.Duration `yaml:"auto_forget_delay"`
	DetailedMetricsEnabled bool          `yaml:"detailed_metrics_enabled"`

	// Wait ring stability.
	WaitStabilityMinDuration time.Duration `yaml:"wait_stability_min_duration"`
	WaitStabilityMaxDuration time.Duration `yaml:"wait_stability_max_duration"`

	// Instance details
	InstanceID             string   `yaml:"instance_id" doc:"hidden"`
	InstanceInterfaceNames []string `yaml:"instance_interface_names"`
	InstancePort           int      `yaml:"instance_port" doc:"hidden"`
	InstanceAddr           string   `yaml:"instance_addr" doc:"hidden"`
	TokensFilePath         string   `yaml:"tokens_file_path"`
	UnregisterOnShutdown   bool     `yaml:"unregister_on_shutdown"`

	// Injected internally
	ListenPort int `yaml:"-"`

	WaitActiveInstanceTimeout time.Duration `yaml:"wait_active_instance_timeout"`

	ObservePeriod time.Duration `yaml:"-"`
}

// RegisterFlags adds the flags required to config this to the given FlagSet
func (cfg *RingConfig) RegisterFlags(f *flag.FlagSet) {
	hostname, err := os.Hostname()
	if err != nil {
		level.Error(util_log.Logger).Log("msg", "failed to get hostname", "err", err)
		os.Exit(1)
	}

	// Ring flags
	cfg.KVStore.RegisterFlagsWithPrefix("compactor.ring.", "collectors/", f)
	f.DurationVar(&cfg.HeartbeatPeriod, "compactor.ring.heartbeat-period", 5*time.Second, "Period at which to heartbeat to the ring. 0 = disabled.")
	f.DurationVar(&cfg.HeartbeatTimeout, "compactor.ring.heartbeat-timeout", time.Minute, "The heartbeat timeout after which compactors are considered unhealthy within the ring. 0 = never (timeout disabled).")
	f.BoolVar(&cfg.DetailedMetricsEnabled, "compactor.ring.detailed-metrics-enabled", true, "Set to true to enable ring detailed metrics. These metrics provide detailed information, such as token count and ownership per tenant. Disabling them can significantly decrease the number of metrics emitted.")
	f.DurationVar(&cfg.AutoForgetDelay, "compactor.auto-forget-delay", 2*cfg.HeartbeatTimeout, "Time since last heartbeat before compactor will be removed from ring. 0 to disable")

	// Wait stability flags.
	f.DurationVar(&cfg.WaitStabilityMinDuration, "compactor.ring.wait-stability-min-duration", time.Minute, "Minimum time to wait for ring stability at startup. 0 to disable.")
	f.DurationVar(&cfg.WaitStabilityMaxDuration, "compactor.ring.wait-stability-max-duration", 5*time.Minute, "Maximum time to wait for ring stability at startup. If the compactor ring keeps changing after this period of time, the compactor will start anyway.")

	// Instance flags
	cfg.InstanceInterfaceNames = []string{"eth0", "en0"}
	f.Var((*flagext.StringSlice)(&cfg.InstanceInterfaceNames), "compactor.ring.instance-interface-names", "Name of network interface to read address from.")
	f.StringVar(&cfg.InstanceAddr, "compactor.ring.instance-addr", "", "IP address to advertise in the ring.")
	f.IntVar(&cfg.InstancePort, "compactor.ring.instance-port", 0, "Port to advertise in the ring (defaults to server.grpc-listen-port).")
	f.StringVar(&cfg.InstanceID, "compactor.ring.instance-id", hostname, "Instance ID to register in the ring.")
	f.StringVar(&cfg.TokensFilePath, "compactor.ring.tokens-file-path", "", "File path where tokens are stored. If empty, tokens are not stored at shutdown and restored at startup.")
	f.BoolVar(&cfg.UnregisterOnShutdown, "compactor.ring.unregister-on-shutdown", true, "Unregister the compactor during shutdown if true.")

	// Timeout durations
	f.DurationVar(&cfg.WaitActiveInstanceTimeout, "compactor.ring.wait-active-instance-timeout", 10*time.Minute, "Timeout for waiting on compactor to become ACTIVE in the ring.")
}

// ToLifecyclerConfig returns a LifecyclerConfig based on the compactor
// ring config.
func (cfg *RingConfig) ToLifecyclerConfig() ring.LifecyclerConfig {
	// We have to make sure that the ring.LifecyclerConfig and ring.Config
	// defaults are preserved
	lc := ring.LifecyclerConfig{}
	rc := ring.Config{}

	flagext.DefaultValues(&lc)
	flagext.DefaultValues(&rc)

	// Configure ring
	rc.KVStore = cfg.KVStore
	rc.HeartbeatTimeout = cfg.HeartbeatTimeout
	rc.ReplicationFactor = 1
	rc.DetailedMetricsEnabled = cfg.DetailedMetricsEnabled

	// Configure lifecycler
	lc.RingConfig = rc
	lc.RingConfig.SubringCacheDisabled = true
	lc.ListenPort = cfg.ListenPort
	lc.Addr = cfg.InstanceAddr
	lc.Port = cfg.InstancePort
	lc.ID = cfg.InstanceID
	lc.InfNames = cfg.InstanceInterfaceNames
	lc.UnregisterOnShutdown = cfg.UnregisterOnShutdown
	lc.HeartbeatPeriod = cfg.HeartbeatPeriod
	lc.ObservePeriod = cfg.ObservePeriod
	lc.JoinAfter = 0
	lc.MinReadyDuration = 0
	lc.FinalSleep = 0
	lc.TokensFilePath = cfg.TokensFilePath

	// We use a safe default instead of exposing to config option to the user
	// in order to simplify the config.
	lc.NumTokens = 512

	return lc
}
