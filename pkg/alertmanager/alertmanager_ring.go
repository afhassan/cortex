package alertmanager

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"

	"github.com/cortexproject/cortex/pkg/ring"
	"github.com/cortexproject/cortex/pkg/ring/kv"
	"github.com/cortexproject/cortex/pkg/util/flagext"
	util_log "github.com/cortexproject/cortex/pkg/util/log"
)

const (
	// RingKey is the key under which we store the alertmanager ring in the KVStore.
	RingKey = "alertmanager"

	// RingNameForServer is the name of the ring used by the alertmanager server.
	RingNameForServer = "alertmanager"

	// RingNumTokens is a safe default instead of exposing to config option to the user
	// in order to simplify the config.
	RingNumTokens = 128
)

// RingOp is the operation used for reading/writing to the alertmanagers.
var RingOp = ring.NewOp([]ring.InstanceState{ring.ACTIVE}, func(s ring.InstanceState) bool {
	// Only ACTIVE Alertmanager get requests. If instance is not ACTIVE, we need to find another Alertmanager.
	return s != ring.ACTIVE
})

// SyncRingOp is the operation used for checking if a user is owned by an alertmanager.
var SyncRingOp = ring.NewOp([]ring.InstanceState{ring.ACTIVE, ring.JOINING}, func(s ring.InstanceState) bool {
	return s != ring.ACTIVE
})

// RingConfig masks the ring lifecycler config which contains
// many options not really required by the alertmanager ring. This config
// is used to strip down the config to the minimum, and avoid confusion
// to the user.
type RingConfig struct {
	KVStore                kv.Config     `yaml:"kvstore" doc:"description=The key-value store used to share the hash ring across multiple instances."`
	HeartbeatPeriod        time.Duration `yaml:"heartbeat_period"`
	HeartbeatTimeout       time.Duration `yaml:"heartbeat_timeout"`
	ReplicationFactor      int           `yaml:"replication_factor"`
	ZoneAwarenessEnabled   bool          `yaml:"zone_awareness_enabled"`
	TokensFilePath         string        `yaml:"tokens_file_path"`
	DetailedMetricsEnabled bool          `yaml:"detailed_metrics_enabled"`

	FinalSleep                      time.Duration `yaml:"final_sleep"`
	WaitInstanceStateTimeout        time.Duration `yaml:"wait_instance_state_timeout"`
	KeepInstanceInTheRingOnShutdown bool          `yaml:"keep_instance_in_the_ring_on_shutdown"`

	// Instance details
	InstanceID             string   `yaml:"instance_id" doc:"hidden"`
	InstanceInterfaceNames []string `yaml:"instance_interface_names"`
	InstancePort           int      `yaml:"instance_port" doc:"hidden"`
	InstanceAddr           string   `yaml:"instance_addr" doc:"hidden"`
	InstanceZone           string   `yaml:"instance_availability_zone"`

	// Injected internally
	ListenPort      int           `yaml:"-"`
	RingCheckPeriod time.Duration `yaml:"-"`

	// Used for testing
	SkipUnregister bool `yaml:"-"`
}

// RegisterFlags adds the flags required to config this to the given FlagSet
func (cfg *RingConfig) RegisterFlags(f *flag.FlagSet) {
	hostname, err := os.Hostname()
	if err != nil {
		level.Error(util_log.Logger).Log("msg", "failed to get hostname", "err", err)
		os.Exit(1)
	}

	// Prefix used by all the ring flags
	rfprefix := "alertmanager.sharding-ring."

	// Ring flags
	cfg.KVStore.RegisterFlagsWithPrefix(rfprefix, "alertmanagers/", f)
	f.DurationVar(&cfg.HeartbeatPeriod, rfprefix+"heartbeat-period", 15*time.Second, "Period at which to heartbeat to the ring. 0 = disabled.")
	f.DurationVar(&cfg.HeartbeatTimeout, rfprefix+"heartbeat-timeout", time.Minute, "The heartbeat timeout after which alertmanagers are considered unhealthy within the ring. 0 = never (timeout disabled).")
	f.DurationVar(&cfg.FinalSleep, rfprefix+"final-sleep", 0*time.Second, "The sleep seconds when alertmanager is shutting down. Need to be close to or larger than KV Store information propagation delay")
	f.IntVar(&cfg.ReplicationFactor, rfprefix+"replication-factor", 3, "The replication factor to use when sharding the alertmanager.")
	f.BoolVar(&cfg.ZoneAwarenessEnabled, rfprefix+"zone-awareness-enabled", false, "True to enable zone-awareness and replicate alerts across different availability zones.")
	f.StringVar(&cfg.TokensFilePath, rfprefix+"tokens-file-path", "", "File path where tokens are stored. If empty, tokens are not stored at shutdown and restored at startup.")
	f.BoolVar(&cfg.DetailedMetricsEnabled, rfprefix+"detailed-metrics-enabled", true, "Set to true to enable ring detailed metrics. These metrics provide detailed information, such as token count and ownership per tenant. Disabling them can significantly decrease the number of metrics emitted.")

	// Instance flags
	cfg.InstanceInterfaceNames = []string{"eth0", "en0"}
	f.Var((*flagext.StringSlice)(&cfg.InstanceInterfaceNames), rfprefix+"instance-interface-names", "Name of network interface to read address from.")
	f.StringVar(&cfg.InstanceAddr, rfprefix+"instance-addr", "", "IP address to advertise in the ring.")
	f.IntVar(&cfg.InstancePort, rfprefix+"instance-port", 0, "Port to advertise in the ring (defaults to server.grpc-listen-port).")
	f.StringVar(&cfg.InstanceID, rfprefix+"instance-id", hostname, "Instance ID to register in the ring.")
	f.StringVar(&cfg.InstanceZone, rfprefix+"instance-availability-zone", "", "The availability zone where this instance is running. Required if zone-awareness is enabled.")
	f.BoolVar(&cfg.KeepInstanceInTheRingOnShutdown, rfprefix+"keep-instance-in-the-ring-on-shutdown", false, "Keep instance in the ring on shut down.")

	cfg.RingCheckPeriod = 5 * time.Second

	// Timeout durations
	f.DurationVar(&cfg.WaitInstanceStateTimeout, rfprefix+"wait-instance-state-timeout", 10*time.Minute, "Timeout for waiting on alertmanager to become desired state in the ring.")
}

// ToLifecyclerConfig returns a LifecyclerConfig based on the alertmanager
// ring config.
func (cfg *RingConfig) ToLifecyclerConfig(logger log.Logger) (ring.BasicLifecyclerConfig, error) {
	instanceAddr, err := ring.GetInstanceAddr(cfg.InstanceAddr, cfg.InstanceInterfaceNames, logger)
	if err != nil {
		return ring.BasicLifecyclerConfig{}, err
	}

	instancePort := ring.GetInstancePort(cfg.InstancePort, cfg.ListenPort)

	return ring.BasicLifecyclerConfig{
		ID:                              cfg.InstanceID,
		Addr:                            fmt.Sprintf("%s:%d", instanceAddr, instancePort),
		HeartbeatPeriod:                 cfg.HeartbeatPeriod,
		TokensObservePeriod:             0,
		Zone:                            cfg.InstanceZone,
		NumTokens:                       RingNumTokens,
		FinalSleep:                      cfg.FinalSleep,
		KeepInstanceInTheRingOnShutdown: cfg.KeepInstanceInTheRingOnShutdown,
	}, nil
}

func (cfg *RingConfig) ToRingConfig() ring.Config {
	rc := ring.Config{}
	flagext.DefaultValues(&rc)

	rc.KVStore = cfg.KVStore
	rc.HeartbeatTimeout = cfg.HeartbeatTimeout
	rc.ReplicationFactor = cfg.ReplicationFactor
	rc.ZoneAwarenessEnabled = cfg.ZoneAwarenessEnabled
	rc.DetailedMetricsEnabled = cfg.DetailedMetricsEnabled

	return rc
}
