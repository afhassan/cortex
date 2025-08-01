package ruler

import (
	"context"
	"flag"
	"fmt"
	"hash/fnv"
	"net/http"
	"net/url"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/pkg/errors"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/rulefmt"
	"github.com/prometheus/prometheus/notifier"
	"github.com/prometheus/prometheus/promql/parser"
	promRules "github.com/prometheus/prometheus/rules"
	"github.com/prometheus/prometheus/util/strutil"
	"github.com/weaveworks/common/user"
	"golang.org/x/sync/errgroup"

	"github.com/cortexproject/cortex/pkg/cortexpb"
	"github.com/cortexproject/cortex/pkg/engine"
	cortexparser "github.com/cortexproject/cortex/pkg/parser"
	"github.com/cortexproject/cortex/pkg/ring"
	"github.com/cortexproject/cortex/pkg/ring/kv"
	"github.com/cortexproject/cortex/pkg/ruler/rulespb"
	"github.com/cortexproject/cortex/pkg/ruler/rulestore"
	"github.com/cortexproject/cortex/pkg/tenant"
	"github.com/cortexproject/cortex/pkg/util"
	util_api "github.com/cortexproject/cortex/pkg/util/api"
	"github.com/cortexproject/cortex/pkg/util/concurrency"
	"github.com/cortexproject/cortex/pkg/util/flagext"
	"github.com/cortexproject/cortex/pkg/util/grpcclient"
	util_log "github.com/cortexproject/cortex/pkg/util/log"
	"github.com/cortexproject/cortex/pkg/util/services"
	"github.com/cortexproject/cortex/pkg/util/validation"
)

var (
	supportedShardingStrategies = []string{util.ShardingStrategyDefault, util.ShardingStrategyShuffle}

	supportedQueryResponseFormats = []string{queryResponseFormatJson, queryResponseFormatProtobuf}

	// Validation errors.
	errInvalidShardingStrategy    = errors.New("invalid sharding strategy")
	errInvalidTenantShardSize     = errors.New("invalid tenant shard size, the value must be greater than 0")
	errInvalidMaxConcurrentEvals  = errors.New("invalid max concurrent evals, the value must be greater than 0")
	errInvalidQueryResponseFormat = errors.New("invalid query response format")
)

const (
	// ringKey is the key under which we store the rulers ring in the KVStore.
	ringKey = "ring"

	// Number of concurrent group list and group loads operations.
	loadRulesConcurrency  = 10
	fetchRulesConcurrency = 16

	rulerSyncReasonInitial    = "initial"
	rulerSyncReasonPeriodic   = "periodic"
	rulerSyncReasonRingChange = "ring-change"

	// Limit errors
	errMaxRuleGroupsPerUserLimitExceeded        = "per-user rule groups limit (limit: %d actual: %d) exceeded"
	errMaxRulesPerRuleGroupPerUserLimitExceeded = "per-user rules per rule group limit (limit: %d actual: %d) exceeded"

	// errors
	errListAllUser = "unable to list the ruler users"

	alertingRuleFilter  string = "alert"
	recordingRuleFilter string = "record"

	firingStateFilter   string = "firing"
	pendingStateFilter  string = "pending"
	inactiveStateFilter string = "inactive"

	unknownHealthFilter string = "unknown"
	okHealthFilter      string = "ok"
	errHealthFilter     string = "err"

	// query response formats
	queryResponseFormatJson     = "json"
	queryResponseFormatProtobuf = "protobuf"
)

type DisabledRuleGroupErr struct {
	Message string
}

func (e *DisabledRuleGroupErr) Error() string {
	return e.Message
}

// Config is the configuration for the recording rules server.
type Config struct {
	// This is used for query to query frontend to evaluate rules
	FrontendAddress string `yaml:"frontend_address"`
	// Query response format of query frontend for evaluating rules
	// It will only take effect FrontendAddress is configured.
	QueryResponseFormat string `yaml:"query_response_format"`
	// HTTP timeout duration when querying to query frontend to evaluate rules
	FrontendTimeout time.Duration `yaml:"-"`
	// Query frontend GRPC Client configuration.
	GRPCClientConfig grpcclient.Config `yaml:"frontend_client"`
	// This is used for template expansion in alerts; must be a valid URL.
	ExternalURL flagext.URLValue `yaml:"external_url"`
	// Labels to add to all alerts
	ExternalLabels labels.Labels `yaml:"external_labels,omitempty" doc:"nocli|description=Labels to add to all alerts."`
	// GRPC Client configuration.
	ClientTLSConfig ClientConfig `yaml:"ruler_client"`
	// How frequently to evaluate rules by default.
	EvaluationInterval time.Duration `yaml:"evaluation_interval"`
	// How frequently to poll for updated rules.
	PollInterval time.Duration `yaml:"poll_interval"`
	// Path to store rule files for prom manager.
	RulePath string `yaml:"rule_path"`

	// URL of the Alertmanager to send notifications to.
	// If you are configuring the ruler to send to a Cortex Alertmanager,
	// ensure this includes any path set in the Alertmanager external URL.
	AlertmanagerURL string `yaml:"alertmanager_url"`
	// Whether to use DNS SRV records to discover Alertmanager.
	AlertmanagerDiscovery bool `yaml:"enable_alertmanager_discovery"`
	// How long to wait between refreshing the list of Alertmanager based on DNS service discovery.
	AlertmanagerRefreshInterval time.Duration `yaml:"alertmanager_refresh_interval"`
	// Capacity of the queue for notifications to be sent to the Alertmanager.
	NotificationQueueCapacity int `yaml:"notification_queue_capacity"`
	// HTTP timeout duration when sending notifications to the Alertmanager.
	NotificationTimeout time.Duration `yaml:"notification_timeout"`
	// Client configs for interacting with the Alertmanager
	Notifier NotifierConfig `yaml:"alertmanager_client"`

	// Max time to tolerate outage for restoring "for" state of alert.
	OutageTolerance time.Duration `yaml:"for_outage_tolerance"`
	// Minimum duration between alert and restored "for" state. This is maintained only for alerts with configured "for" time greater than grace period.
	ForGracePeriod time.Duration `yaml:"for_grace_period"`
	// Minimum amount of time to wait before resending an alert to Alertmanager.
	ResendDelay time.Duration `yaml:"resend_delay"`

	ConcurrentEvalsEnabled bool  `yaml:"concurrent_evals_enabled"`
	MaxConcurrentEvals     int64 `yaml:"max_concurrent_evals"`

	// Enable sharding rule groups.
	EnableSharding   bool          `yaml:"enable_sharding"`
	ShardingStrategy string        `yaml:"sharding_strategy"`
	SearchPendingFor time.Duration `yaml:"search_pending_for"`
	Ring             RingConfig    `yaml:"ring"`
	FlushCheckPeriod time.Duration `yaml:"flush_period"`

	EnableAPI           bool `yaml:"enable_api"`
	APIDeduplicateRules bool `yaml:"api_deduplicate_rules"`

	EnabledTenants  flagext.StringSliceCSV `yaml:"enabled_tenants"`
	DisabledTenants flagext.StringSliceCSV `yaml:"disabled_tenants"`

	RingCheckPeriod time.Duration `yaml:"-"`

	// Field will be populated during runtime.
	LookbackDelta        time.Duration `yaml:"-"`
	PrometheusHTTPPrefix string        `yaml:"-"`

	EnableQueryStats      bool `yaml:"query_stats_enabled"`
	DisableRuleGroupLabel bool `yaml:"disable_rule_group_label"`

	EnableHAEvaluation   bool          `yaml:"enable_ha_evaluation"`
	LivenessCheckTimeout time.Duration `yaml:"liveness_check_timeout"`

	ThanosEngine engine.ThanosEngineConfig `yaml:"thanos_engine"`
}

// Validate config and returns error on failure
func (cfg *Config) Validate(limits validation.Limits, log log.Logger) error {
	if !util.StringsContain(supportedShardingStrategies, cfg.ShardingStrategy) {
		return errInvalidShardingStrategy
	}

	if cfg.ShardingStrategy == util.ShardingStrategyShuffle && limits.RulerTenantShardSize <= 0 {
		return errInvalidTenantShardSize
	}

	if err := cfg.ClientTLSConfig.Validate(log); err != nil {
		return errors.Wrap(err, "invalid ruler gRPC client config")
	}

	if err := cfg.GRPCClientConfig.Validate(log); err != nil {
		return errors.Wrap(err, "invalid query frontend gRPC client config")
	}

	if cfg.ConcurrentEvalsEnabled && cfg.MaxConcurrentEvals <= 0 {
		return errInvalidMaxConcurrentEvals
	}

	if !util.StringsContain(supportedQueryResponseFormats, cfg.QueryResponseFormat) {
		return errInvalidQueryResponseFormat
	}

	if err := cfg.ThanosEngine.Validate(); err != nil {
		return err
	}

	return nil
}

// RegisterFlags adds the flags required to config this to the given FlagSet
func (cfg *Config) RegisterFlags(f *flag.FlagSet) {
	cfg.ClientTLSConfig.RegisterFlagsWithPrefix("ruler.client", f)
	cfg.GRPCClientConfig.RegisterFlagsWithPrefix("ruler.frontendClient", "", f)
	cfg.Ring.RegisterFlags(f)
	cfg.Notifier.RegisterFlags(f)
	cfg.ThanosEngine.RegisterFlagsWithPrefix("ruler.", f)

	// Deprecated Flags that will be maintained to avoid user disruption

	//lint:ignore faillint Need to pass the global logger like this for warning on deprecated methods
	flagext.DeprecatedFlag(f, "ruler.client-timeout", "This flag has been renamed to ruler.configs.client-timeout", util_log.Logger)
	//lint:ignore faillint Need to pass the global logger like this for warning on deprecated methods
	flagext.DeprecatedFlag(f, "ruler.group-timeout", "This flag is no longer functional.", util_log.Logger)
	//lint:ignore faillint Need to pass the global logger like this for warning on deprecated methods
	flagext.DeprecatedFlag(f, "ruler.num-workers", "This flag is no longer functional. For increased concurrency horizontal sharding is recommended", util_log.Logger)
	//lint:ignore faillint Need to pass the global logger like this for warning on deprecated methods
	flagext.DeprecatedFlag(f, "ruler.alertmanager-use-v2", "This flag is no longer functional. V1 API is deprecated and removed", util_log.Logger)

	f.StringVar(&cfg.FrontendAddress, "ruler.frontend-address", "", "[Experimental] GRPC listen address of the Query Frontend, in host:port format. If set, Ruler queries to Query Frontends via gRPC. If not set, ruler queries to Ingesters directly.")
	f.StringVar(&cfg.QueryResponseFormat, "ruler.query-response-format", queryResponseFormatProtobuf, fmt.Sprintf("[Experimental] Query response format to get query results from Query Frontend when the rule evaluation. It will only take effect when `-ruler.frontend-address` is configured. Supported values: %s", strings.Join(supportedQueryResponseFormats, ",")))
	cfg.ExternalURL.URL, _ = url.Parse("") // Must be non-nil
	f.Var(&cfg.ExternalURL, "ruler.external.url", "URL of alerts return path.")
	f.DurationVar(&cfg.EvaluationInterval, "ruler.evaluation-interval", 1*time.Minute, "How frequently to evaluate rules")
	f.DurationVar(&cfg.PollInterval, "ruler.poll-interval", 1*time.Minute, "How frequently to poll for rule changes")

	f.StringVar(&cfg.AlertmanagerURL, "ruler.alertmanager-url", "", "Comma-separated list of URL(s) of the Alertmanager(s) to send notifications to. Each Alertmanager URL is treated as a separate group in the configuration. Multiple Alertmanagers in HA per group can be supported by using DNS resolution via -ruler.alertmanager-discovery.")
	f.BoolVar(&cfg.AlertmanagerDiscovery, "ruler.alertmanager-discovery", false, "Use DNS SRV records to discover Alertmanager hosts.")
	f.DurationVar(&cfg.AlertmanagerRefreshInterval, "ruler.alertmanager-refresh-interval", 1*time.Minute, "How long to wait between refreshing DNS resolutions of Alertmanager hosts.")
	f.IntVar(&cfg.NotificationQueueCapacity, "ruler.notification-queue-capacity", 10000, "Capacity of the queue for notifications to be sent to the Alertmanager.")
	f.DurationVar(&cfg.NotificationTimeout, "ruler.notification-timeout", 10*time.Second, "HTTP timeout duration when sending notifications to the Alertmanager.")

	f.DurationVar(&cfg.SearchPendingFor, "ruler.search-pending-for", 5*time.Minute, "Time to spend searching for a pending ruler when shutting down.")
	f.BoolVar(&cfg.EnableSharding, "ruler.enable-sharding", false, "Distribute rule evaluation using ring backend")
	f.StringVar(&cfg.ShardingStrategy, "ruler.sharding-strategy", util.ShardingStrategyDefault, fmt.Sprintf("The sharding strategy to use. Supported values are: %s.", strings.Join(supportedShardingStrategies, ", ")))
	f.DurationVar(&cfg.FlushCheckPeriod, "ruler.flush-period", 1*time.Minute, "Period with which to attempt to flush rule groups.")
	f.StringVar(&cfg.RulePath, "ruler.rule-path", "/rules", "file path to store temporary rule files for the prometheus rule managers")
	f.BoolVar(&cfg.EnableAPI, "experimental.ruler.enable-api", false, "Enable the ruler api")
	f.BoolVar(&cfg.APIDeduplicateRules, "experimental.ruler.api-deduplicate-rules", false, "EXPERIMENTAL: Remove duplicate rules in the prometheus rules and alerts API response. If there are duplicate rules the rule with the latest evaluation timestamp will be kept.")
	f.DurationVar(&cfg.OutageTolerance, "ruler.for-outage-tolerance", time.Hour, `Max time to tolerate outage for restoring "for" state of alert.`)
	f.DurationVar(&cfg.ForGracePeriod, "ruler.for-grace-period", 10*time.Minute, `Minimum duration between alert and restored "for" state. This is maintained only for alerts with configured "for" time greater than grace period.`)
	f.DurationVar(&cfg.ResendDelay, "ruler.resend-delay", time.Minute, `Minimum amount of time to wait before resending an alert to Alertmanager.`)
	f.BoolVar(&cfg.ConcurrentEvalsEnabled, "ruler.concurrent-evals-enabled", false, `If enabled, rules from a single rule group can be evaluated concurrently if there is no dependency between each other. Max concurrency for each rule group is controlled via ruler.max-concurrent-evals flag.`)
	f.Int64Var(&cfg.MaxConcurrentEvals, "ruler.max-concurrent-evals", 1, `Max concurrency for a single rule group to evaluate independent rules.`)

	f.Var(&cfg.EnabledTenants, "ruler.enabled-tenants", "Comma separated list of tenants whose rules this ruler can evaluate. If specified, only these tenants will be handled by ruler, otherwise this ruler can process rules from all tenants. Subject to sharding.")
	f.Var(&cfg.DisabledTenants, "ruler.disabled-tenants", "Comma separated list of tenants whose rules this ruler cannot evaluate. If specified, a ruler that would normally pick the specified tenant(s) for processing will ignore them instead. Subject to sharding.")

	f.BoolVar(&cfg.EnableQueryStats, "ruler.query-stats-enabled", false, "Report query statistics for ruler queries to complete as a per user metric and as an info level log message.")
	f.BoolVar(&cfg.DisableRuleGroupLabel, "ruler.disable-rule-group-label", false, "Disable the rule_group label on exported metrics")

	f.BoolVar(&cfg.EnableHAEvaluation, "ruler.enable-ha-evaluation", false, "Enable high availability")
	f.DurationVar(&cfg.LivenessCheckTimeout, "ruler.liveness-check-timeout", 1*time.Second, "Timeout duration for non-primary rulers during liveness checks. If the check times out, the non-primary ruler will evaluate the rule group. Applicable when ruler.enable-ha-evaluation is true.")
	cfg.RingCheckPeriod = 5 * time.Second
}

func (cfg *Config) RulesBackupEnabled() bool {
	// If the replication factor is greater the  1, only the first replica is responsible for evaluating the rule,
	// the rest of the replica will store the rule groups as backup only for API HA.
	return cfg.Ring.ReplicationFactor > 1
}

// MultiTenantManager is the interface of interaction with a Manager that is tenant aware.
type MultiTenantManager interface {
	// SyncRuleGroups is used to sync the Manager with rules from the RuleStore.
	// If existing user is missing in the ruleGroups map, its ruler manager will be stopped.
	SyncRuleGroups(ctx context.Context, ruleGroups map[string]rulespb.RuleGroupList)
	// BackUpRuleGroups is used to store backups of rule groups owned by a different ruler instance.
	BackUpRuleGroups(ctx context.Context, ruleGroups map[string]rulespb.RuleGroupList)
	// GetRules fetches rules for a particular tenant (userID).
	GetRules(userID string) []*promRules.Group
	// GetBackupRules fetches rules for a particular tenant (userID) that the ruler stores for backup purposes
	GetBackupRules(userID string) rulespb.RuleGroupList
	// Stop stops all Manager components.
	Stop()
	// ValidateRuleGroup validates a rulegroup
	ValidateRuleGroup(rulefmt.RuleGroup) []error
}

// Ruler evaluates rules.
//
//	+---------------------------------------------------------------+
//	|                                                               |
//	|                   Query       +-------------+                 |
//	|            +------------------>             |                 |
//	|            |                  |    Store    |                 |
//	|            | +----------------+             |                 |
//	|            | |     Rules      +-------------+                 |
//	|            | |                                                |
//	|            | |                                                |
//	|            | |                                                |
//	|       +----+-v----+   Filter  +------------+                  |
//	|       |           +----------->            |                  |
//	|       |   Ruler   |           |    Ring    |                  |
//	|       |           <-----------+            |                  |
//	|       +-------+---+   Rules   +------------+                  |
//	|               |                                               |
//	|               |                                               |
//	|               |                                               |
//	|               |    Load      +-----------------+              |
//	|               +-------------->                 |              |
//	|                              |     Manager     |              |
//	|                              |                 |              |
//	|                              +-----------------+              |
//	|                                                               |
//	+---------------------------------------------------------------+
type Ruler struct {
	services.Service

	cfg        Config
	lifecycler *ring.BasicLifecycler
	ring       *ring.Ring
	store      rulestore.RuleStore
	manager    MultiTenantManager
	limits     RulesLimits

	subservices        *services.Manager
	subservicesWatcher *services.FailureWatcher

	// Pool of clients used to connect to other ruler replicas.
	clientsPool ClientsPool

	ringCheckErrors            prometheus.Counter
	rulerSync                  *prometheus.CounterVec
	ruleGroupStoreLoadDuration prometheus.Gauge
	ruleGroupSyncDuration      prometheus.Gauge
	rulerGetRulesFailures      *prometheus.CounterVec
	ruleGroupMetrics           *RuleGroupMetrics

	allowedTenants *util.AllowedTenants

	registry prometheus.Registerer
	logger   log.Logger
}

// NewRuler creates a new ruler from a distributor and chunk store.
func NewRuler(cfg Config, manager MultiTenantManager, reg prometheus.Registerer, logger log.Logger, ruleStore rulestore.RuleStore, limits RulesLimits) (*Ruler, error) {
	return newRuler(cfg, manager, reg, logger, ruleStore, limits, newRulerClientPool(cfg.ClientTLSConfig.Config, logger, reg))
}

func newRuler(cfg Config, manager MultiTenantManager, reg prometheus.Registerer, logger log.Logger, ruleStore rulestore.RuleStore, limits RulesLimits, clientPool ClientsPool) (*Ruler, error) {
	ruler := &Ruler{
		cfg:            cfg,
		store:          ruleStore,
		manager:        manager,
		registry:       reg,
		logger:         logger,
		limits:         limits,
		clientsPool:    clientPool,
		allowedTenants: util.NewAllowedTenants(cfg.EnabledTenants, cfg.DisabledTenants),

		ringCheckErrors: promauto.With(reg).NewCounter(prometheus.CounterOpts{
			Name: "cortex_ruler_ring_check_errors_total",
			Help: "Number of errors that have occurred when checking the ring for ownership",
		}),

		rulerSync: promauto.With(reg).NewCounterVec(prometheus.CounterOpts{
			Name: "cortex_ruler_sync_rules_total",
			Help: "Total number of times the ruler sync operation triggered.",
		}, []string{"reason"}),

		ruleGroupStoreLoadDuration: promauto.With(reg).NewGauge(prometheus.GaugeOpts{
			Name: "cortex_ruler_rule_group_load_duration_seconds",
			Help: "Time taken to load rule groups from storage",
		}),

		ruleGroupSyncDuration: promauto.With(reg).NewGauge(prometheus.GaugeOpts{
			Name: "cortex_ruler_rule_group_sync_duration_seconds",
			Help: "The duration in seconds required to sync and load rule groups from storage.",
		}),

		rulerGetRulesFailures: promauto.With(reg).NewCounterVec(prometheus.CounterOpts{
			Name: "cortex_ruler_get_rules_failure_total",
			Help: "The total number of failed rules request sent to rulers in getShardedRules.",
		}, []string{"ruler"}),
	}
	ruler.ruleGroupMetrics = NewRuleGroupMetrics(reg, ruler.allowedTenants)

	if len(cfg.EnabledTenants) > 0 {
		level.Info(ruler.logger).Log("msg", "ruler using enabled users", "enabled", strings.Join(cfg.EnabledTenants, ", "))
	}
	if len(cfg.DisabledTenants) > 0 {
		level.Info(ruler.logger).Log("msg", "ruler using disabled users", "disabled", strings.Join(cfg.DisabledTenants, ", "))
	}

	if cfg.EnableSharding {
		ringStore, err := kv.NewClient(
			cfg.Ring.KVStore,
			ring.GetCodec(),
			kv.RegistererWithKVName(prometheus.WrapRegistererWithPrefix("cortex_", reg), "ruler"),
			logger,
		)
		if err != nil {
			return nil, errors.Wrap(err, "create KV store client")
		}

		if err = enableSharding(ruler, ringStore); err != nil {
			return nil, errors.Wrap(err, "setup ruler sharding ring")
		}
	}

	ruler.Service = services.NewBasicService(ruler.starting, ruler.run, ruler.stopping)
	return ruler, nil
}

func enableSharding(r *Ruler, ringStore kv.Client) error {
	lifecyclerCfg, err := r.cfg.Ring.ToLifecyclerConfig(r.logger)
	if err != nil {
		return errors.Wrap(err, "failed to initialize ruler's lifecycler config")
	}

	// Define lifecycler delegates in reverse order (last to be called defined first because they're
	// chained via "next delegate").
	delegate := ring.BasicLifecyclerDelegate(r)
	if !r.Config().Ring.KeepInstanceInTheRingOnShutdown {
		delegate = ring.NewLeaveOnStoppingDelegate(delegate, r.logger)
	}
	delegate = ring.NewTokensPersistencyDelegate(r.cfg.Ring.TokensFilePath, ring.JOINING, delegate, r.logger)
	delegate = ring.NewAutoForgetDelegate(r.cfg.Ring.HeartbeatTimeout*ringAutoForgetUnhealthyPeriods, delegate, r.logger)

	rulerRingName := "ruler"
	r.lifecycler, err = ring.NewBasicLifecycler(lifecyclerCfg, rulerRingName, ringKey, ringStore, delegate, r.logger, prometheus.WrapRegistererWithPrefix("cortex_", r.registry))
	if err != nil {
		return errors.Wrap(err, "failed to initialize ruler's lifecycler")
	}

	r.ring, err = ring.NewWithStoreClientAndStrategy(r.cfg.Ring.ToRingConfig(), rulerRingName, ringKey, ringStore, ring.NewIgnoreUnhealthyInstancesReplicationStrategy(), prometheus.WrapRegistererWithPrefix("cortex_", r.registry), r.logger)
	if err != nil {
		return errors.Wrap(err, "failed to initialize ruler's ring")
	}

	return nil
}

func (r *Ruler) Logger() log.Logger {
	return r.logger
}

func (r *Ruler) GetClientFor(addr string) (RulerClient, error) {
	return r.clientsPool.GetClientFor(addr)
}

func (r *Ruler) Config() Config {
	return r.cfg
}

func (r *Ruler) starting(ctx context.Context) error {
	// If sharding is enabled, start the used subservices.
	if r.cfg.EnableSharding {
		var err error

		if r.subservices, err = services.NewManager(r.lifecycler, r.ring, r.clientsPool); err != nil {
			return errors.Wrap(err, "unable to start ruler subservices")
		}

		r.subservicesWatcher = services.NewFailureWatcher()
		r.subservicesWatcher.WatchManager(r.subservices)

		if err = services.StartManagerAndAwaitHealthy(ctx, r.subservices); err != nil {
			return errors.Wrap(err, "unable to start ruler subservices")
		}
	}

	// TODO: ideally, ruler would wait until its queryable is finished starting.
	return nil
}

// Stop stops the Ruler.
// Each function of the ruler is terminated before leaving the ring
func (r *Ruler) stopping(_ error) error {
	r.manager.Stop()

	if r.subservices != nil {
		_ = services.StopManagerAndAwaitStopped(context.Background(), r.subservices)
	}
	return nil
}

type sender interface {
	Send(alerts ...*notifier.Alert)
}

// SendAlerts implements a rules.NotifyFunc for a Notifier.
// It filters any non-firing alerts from the input.
//
// Copied from Prometheus's main.go.
func SendAlerts(n sender, externalURL string) promRules.NotifyFunc {
	return func(ctx context.Context, expr string, alerts ...*promRules.Alert) {
		var res []*notifier.Alert

		for _, alert := range alerts {
			a := &notifier.Alert{
				StartsAt:     alert.FiredAt,
				Labels:       alert.Labels,
				Annotations:  alert.Annotations,
				GeneratorURL: externalURL + strutil.TableLinkForExpression(expr),
			}
			if !alert.ResolvedAt.IsZero() {
				a.EndsAt = alert.ResolvedAt
			} else {
				a.EndsAt = alert.ValidUntil
			}
			res = append(res, a)
		}

		if len(alerts) > 0 {
			n.Send(res...)
		}
	}
}

func ruleGroupDisabled(ruleGroup *rulespb.RuleGroupDesc, disabledRuleGroupsForUser validation.DisabledRuleGroups) bool {
	for _, disabledRuleGroupForUser := range disabledRuleGroupsForUser {
		if ruleGroup.Namespace == disabledRuleGroupForUser.Namespace &&
			ruleGroup.Name == disabledRuleGroupForUser.Name &&
			ruleGroup.User == disabledRuleGroupForUser.User {
			return true
		}
	}
	return false
}

var sep = []byte("/")

func tokenForGroup(g *rulespb.RuleGroupDesc) uint32 {
	ringHasher := fnv.New32a()

	// Hasher never returns err.
	_, _ = ringHasher.Write([]byte(g.User))
	_, _ = ringHasher.Write(sep)
	_, _ = ringHasher.Write([]byte(g.Namespace))
	_, _ = ringHasher.Write(sep)
	_, _ = ringHasher.Write([]byte(g.Name))

	return ringHasher.Sum32()
}

func (r *Ruler) instanceOwnsRuleGroup(rr ring.ReadRing, g *rulespb.RuleGroupDesc, disabledRuleGroups validation.DisabledRuleGroups, forBackup bool) (bool, error) {

	hash := tokenForGroup(g)

	rlrs, err := rr.Get(hash, RingOp, nil, nil, nil)
	if err != nil {
		return false, errors.Wrap(err, "error reading ring to verify rule group ownership")
	}

	instanceAddr := r.lifecycler.GetInstanceAddr()
	if forBackup {
		// Only the second up to the last replica are used as backup
		for i := 1; i < len(rlrs.Instances); i++ {
			if rlrs.Instances[i].Addr == instanceAddr {
				return ownsRuleGroupOrDisable(g, disabledRuleGroups)
			}
		}
		return false, nil
	}
	if r.Config().EnableHAEvaluation {
		for i, ruler := range rlrs.Instances {
			if ruler.Addr == instanceAddr && i == 0 {
				level.Debug(r.Logger()).Log("msg", "primary taking ownership", "user", g.User, "group", g.Name, "namespace", g.Namespace, "ruler", instanceAddr)
				return ownsRuleGroupOrDisable(g, disabledRuleGroups)
			}
			if ruler.Addr == instanceAddr && r.nonPrimaryInstanceOwnsRuleGroup(g, rlrs.GetAddresses()[:i]) {
				level.Info(r.Logger()).Log("msg", "non-primary ruler taking ownership", "user", g.User, "group", g.Name, "namespace", g.Namespace, "ruler", instanceAddr)
				return ownsRuleGroupOrDisable(g, disabledRuleGroups)
			}
		}
		return false, nil
	}
	// Even if the replication factor is set to a number bigger than 1, only the first ruler evaluates the rule group
	if rlrs.Instances[0].Addr == instanceAddr {
		return ownsRuleGroupOrDisable(g, disabledRuleGroups)
	}
	return false, nil
}

func ownsRuleGroupOrDisable(g *rulespb.RuleGroupDesc, disabledRuleGroups validation.DisabledRuleGroups) (bool, error) {
	if ruleGroupDisabled(g, disabledRuleGroups) {
		return false, &DisabledRuleGroupErr{Message: fmt.Sprintf("rule group %s, namespace %s, user %s is disabled", g.Name, g.Namespace, g.User)}
	}
	return true, nil
}

func (r *Ruler) LivenessCheck(_ context.Context, request *LivenessCheckRequest) (*LivenessCheckResponse, error) {
	if r.lifecycler.ServiceContext() == nil {
		return nil, errors.New("ruler is not yet ready")
	}
	if r.lifecycler.ServiceContext().Err() != nil || r.subservices.IsStopped() {
		return nil, errors.New("ruler's context is canceled and might be stopping soon")
	}
	if !r.subservices.IsHealthy() {
		return nil, errors.New("not all subservices are in healthy state")
	}
	return &LivenessCheckResponse{State: int32(r.State())}, nil
}

// This function performs a liveness check against the provided replicas. If any one of the replicas responds with a state = Running, then
// this Ruler should not take ownership of the rule group. Otherwise, this Ruler must take ownership of the rule group to avoid missing evaluations
func (r *Ruler) nonPrimaryInstanceOwnsRuleGroup(g *rulespb.RuleGroupDesc, replicas []string) bool {
	userID := g.User

	jobs := concurrency.CreateJobsFromStrings(replicas)

	errorChan := make(chan error, len(jobs))
	responseChan := make(chan *LivenessCheckResponse, len(jobs))

	ctx := user.InjectOrgID(context.Background(), userID)
	ctx, cancel := context.WithTimeout(ctx, r.cfg.LivenessCheckTimeout)
	defer cancel()

	err := concurrency.ForEach(ctx, jobs, len(jobs), func(ctx context.Context, job interface{}) error {
		addr := job.(string)
		rulerClient, err := r.GetClientFor(addr)
		if err != nil {
			errorChan <- err
			level.Error(r.Logger()).Log("msg", "unable to get client for ruler", "ruler addr", addr)
			return nil
		}
		level.Debug(r.Logger()).Log("msg", "performing liveness check against", "addr", addr, "for", g.Name)

		resp, err := rulerClient.LivenessCheck(ctx, &LivenessCheckRequest{})
		if err != nil {
			errorChan <- err
			level.Debug(r.Logger()).Log("msg", "liveness check failed", "addr", addr, "for", g.Name, "err", err.Error())
			return nil
		}
		level.Debug(r.Logger()).Log("msg", "liveness check succeeded ", "addr", addr, "for", g.Name, "ruler state", services.State(resp.GetState()))
		responseChan <- resp
		return nil
	})

	close(errorChan)
	close(responseChan)

	if len(errorChan) == len(jobs) || err != nil {
		return true
	}

	for resp := range responseChan {
		if services.State(resp.GetState()) == services.Running {
			return false
		}
	}
	return true
}

func (r *Ruler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if r.cfg.EnableSharding {
		r.ring.ServeHTTP(w, req)
	} else {
		var unshardedPage = `
			<!DOCTYPE html>
			<html>
				<head>
					<meta charset="UTF-8">
					<title>Cortex Ruler Status</title>
				</head>
				<body>
					<h1>Cortex Ruler Status</h1>
					<p>Ruler running with shards disabled</p>
				</body>
			</html>`
		util.WriteHTMLResponse(w, unshardedPage)
	}
}

func (r *Ruler) run(ctx context.Context) error {
	level.Info(r.logger).Log("msg", "ruler up and running")

	tick := time.NewTicker(r.cfg.PollInterval)
	defer tick.Stop()

	var ringTickerChan <-chan time.Time
	var ringLastState ring.ReplicationSet

	if r.cfg.EnableSharding {
		ringLastState, _ = r.ring.GetAllHealthy(RingOp)
		ringTicker := time.NewTicker(util.DurationWithJitter(r.cfg.RingCheckPeriod, 0.2))
		defer ringTicker.Stop()
		ringTickerChan = ringTicker.C
	}

	syncRuleErrMsg := func(syncRulesErr error) {
		level.Error(r.logger).Log("msg", "failed to sync rules", "err", syncRulesErr)
	}

	initialSyncErr := r.syncRules(ctx, rulerSyncReasonInitial)
	if initialSyncErr != nil {
		syncRuleErrMsg(initialSyncErr)
	}
	for {
		var syncRulesErr error
		select {
		case <-ctx.Done():
			return nil
		case <-tick.C:
			syncRulesErr = r.syncRules(ctx, rulerSyncReasonPeriodic)
		case <-ringTickerChan:
			// We ignore the error because in case of error it will return an empty
			// replication set which we use to compare with the previous state.
			currRingState, _ := r.ring.GetAllHealthy(RingOp)

			if ring.HasReplicationSetChanged(ringLastState, currRingState) {
				ringLastState = currRingState
				syncRulesErr = r.syncRules(ctx, rulerSyncReasonRingChange)
			}
		case err := <-r.subservicesWatcher.Chan():
			return errors.Wrap(err, "ruler subservice failed")
		}
		if syncRulesErr != nil {
			syncRuleErrMsg(syncRulesErr)
		}
	}
}

func (r *Ruler) syncRules(ctx context.Context, reason string) error {
	level.Info(r.logger).Log("msg", "syncing rules", "reason", reason)
	r.rulerSync.WithLabelValues(reason).Inc()
	timer := prometheus.NewTimer(nil)

	defer func() {
		ruleGroupSyncDuration := timer.ObserveDuration().Seconds()
		r.ruleGroupSyncDuration.Set(ruleGroupSyncDuration)
	}()

	loadedConfigs, backupConfigs, err := r.loadRuleGroups(ctx)
	if err != nil {
		return err
	}

	if ctx.Err() != nil {
		level.Info(r.logger).Log("msg", "context is canceled. not syncing rules")
		return err
	}
	// This will also delete local group files for users that are no longer in 'configs' map.
	r.manager.SyncRuleGroups(ctx, loadedConfigs)

	if r.cfg.RulesBackupEnabled() {
		r.manager.BackUpRuleGroups(ctx, backupConfigs)
	}

	return nil
}

func (r *Ruler) loadRuleGroups(ctx context.Context) (map[string]rulespb.RuleGroupList, map[string]rulespb.RuleGroupList, error) {
	timer := prometheus.NewTimer(nil)

	defer func() {
		storeLoadSeconds := timer.ObserveDuration().Seconds()
		r.ruleGroupStoreLoadDuration.Set(storeLoadSeconds)
	}()

	ownedConfigs, backupConfigs, err := r.listRules(ctx)
	if err != nil {
		level.Error(r.logger).Log("msg", "unable to list rules", "err", err)
		return nil, nil, err
	}

	loadedOwnedConfigs, err := r.store.LoadRuleGroups(ctx, ownedConfigs)
	if err != nil {
		level.Warn(r.logger).Log("msg", "failed to load some rules owned by this ruler", "count", len(ownedConfigs)-len(loadedOwnedConfigs), "err", err)
	}
	if r.cfg.RulesBackupEnabled() {
		loadedBackupConfigs, err := r.store.LoadRuleGroups(ctx, backupConfigs)
		if err != nil {
			level.Warn(r.logger).Log("msg", "failed to load some rules backed up by this ruler", "count", len(backupConfigs)-len(loadedBackupConfigs), "err", err)
		}
		return loadedOwnedConfigs, loadedBackupConfigs, nil
	}
	return loadedOwnedConfigs, nil, nil
}

func (r *Ruler) listRules(ctx context.Context) (owned map[string]rulespb.RuleGroupList, backedUp map[string]rulespb.RuleGroupList, err error) {
	switch {
	case !r.cfg.EnableSharding:
		owned, backedUp, err = r.listRulesNoSharding(ctx)

	case r.cfg.ShardingStrategy == util.ShardingStrategyDefault:
		owned, backedUp, err = r.listRulesShardingDefault(ctx)

	case r.cfg.ShardingStrategy == util.ShardingStrategyShuffle:
		owned, backedUp, err = r.listRulesShuffleSharding(ctx)

	default:
		return nil, nil, errors.New("invalid sharding configuration")
	}

	if err != nil {
		return
	}

	for userID := range owned {
		if !r.allowedTenants.IsAllowed(userID) {
			level.Debug(r.logger).Log("msg", "ignoring rule groups for user, not allowed", "user", userID)
			delete(owned, userID)
		}
	}

	for userID := range backedUp {
		if !r.allowedTenants.IsAllowed(userID) {
			level.Debug(r.logger).Log("msg", "ignoring rule groups for user, not allowed", "user", userID)
			delete(backedUp, userID)
		}
	}
	return
}

func (r *Ruler) listRulesNoSharding(ctx context.Context) (map[string]rulespb.RuleGroupList, map[string]rulespb.RuleGroupList, error) {
	allRuleGroups, err := r.store.ListAllRuleGroups(ctx)
	if err != nil {
		return nil, nil, err
	}
	ruleGroupCounts := make(map[string]int, len(allRuleGroups))
	for userID, groups := range allRuleGroups {
		ruleGroupCounts[userID] = len(groups)
		disabledRuleGroupsForUser := r.limits.DisabledRuleGroups(userID)
		if len(disabledRuleGroupsForUser) == 0 {
			continue
		}
		filteredGroupsForUser := rulespb.RuleGroupList{}
		for _, group := range groups {
			if !ruleGroupDisabled(group, disabledRuleGroupsForUser) {
				filteredGroupsForUser = append(filteredGroupsForUser, group)
			} else {
				level.Info(r.logger).Log("msg", "rule group disabled", "name", group.Name, "namespace", group.Namespace, "user", group.User)
			}
		}
		allRuleGroups[userID] = filteredGroupsForUser
	}
	r.ruleGroupMetrics.UpdateRuleGroupsInStore(ruleGroupCounts)
	return allRuleGroups, nil, nil
}

func (r *Ruler) listRulesShardingDefault(ctx context.Context) (map[string]rulespb.RuleGroupList, map[string]rulespb.RuleGroupList, error) {
	configs, err := r.store.ListAllRuleGroups(ctx)
	if err != nil {
		return nil, nil, err
	}

	ruleGroupCounts := make(map[string]int, len(configs))
	ownedConfigs := make(map[string]rulespb.RuleGroupList)
	backedUpConfigs := make(map[string]rulespb.RuleGroupList)
	for userID, groups := range configs {
		ruleGroupCounts[userID] = len(groups)
		owned := r.filterRuleGroups(userID, groups, r.ring)
		if len(owned) > 0 {
			ownedConfigs[userID] = owned
		}
		if r.cfg.RulesBackupEnabled() {
			backup := r.filterBackupRuleGroups(userID, groups, owned, r.ring)
			if len(backup) > 0 {
				backedUpConfigs[userID] = backup
			}
		}
	}
	r.ruleGroupMetrics.UpdateRuleGroupsInStore(ruleGroupCounts)
	return ownedConfigs, backedUpConfigs, nil
}

func (r *Ruler) listRulesShuffleSharding(ctx context.Context) (map[string]rulespb.RuleGroupList, map[string]rulespb.RuleGroupList, error) {
	users, err := r.store.ListAllUsers(ctx)
	if err != nil {
		return nil, nil, errors.Wrap(err, "unable to list users of ruler")
	}

	// Only users in userRings will be used in the to load the rules.
	userRings := map[string]ring.ReadRing{}
	for _, u := range users {
		if shardSize := r.limits.RulerTenantShardSize(u); shardSize > 0 {
			subRing := r.ring.ShuffleShard(u, r.getShardSizeForUser(u))

			// Include the user only if it belongs to this ruler shard.
			if subRing.HasInstance(r.lifecycler.GetInstanceID()) {
				userRings[u] = subRing
			}
		} else {
			// A shard size of 0 means shuffle sharding is disabled for this specific user.
			// In that case we use the full ring so that rule groups will be sharded across all rulers.
			userRings[u] = r.ring
		}
	}

	if len(userRings) == 0 {
		r.ruleGroupMetrics.UpdateRuleGroupsInStore(make(map[string]int))
		return nil, nil, nil
	}

	userCh := make(chan string, len(userRings))
	for u := range userRings {
		userCh <- u
	}
	close(userCh)

	mu := sync.Mutex{}
	owned := map[string]rulespb.RuleGroupList{}
	backedUp := map[string]rulespb.RuleGroupList{}
	gLock := sync.Mutex{}
	ruleGroupCounts := make(map[string]int, len(userRings))

	concurrency := loadRulesConcurrency
	if len(userRings) < concurrency {
		concurrency = len(userRings)
	}

	g, gctx := errgroup.WithContext(ctx)
	for i := 0; i < concurrency; i++ {
		g.Go(func() error {
			for userID := range userCh {
				groups, err := r.store.ListRuleGroupsForUserAndNamespace(gctx, userID, "")
				if err != nil {
					return errors.Wrapf(err, "failed to fetch rule groups for user %s", userID)
				}
				gLock.Lock()
				ruleGroupCounts[userID] = len(groups)
				gLock.Unlock()

				filterOwned := r.filterRuleGroups(userID, groups, userRings[userID])
				var filterBackup []*rulespb.RuleGroupDesc
				if r.cfg.RulesBackupEnabled() {
					filterBackup = r.filterBackupRuleGroups(userID, groups, filterOwned, userRings[userID])
				}
				if len(filterOwned) == 0 && len(filterBackup) == 0 {
					continue
				}
				mu.Lock()
				if len(filterOwned) > 0 {
					owned[userID] = filterOwned
				}
				if len(filterBackup) > 0 {
					backedUp[userID] = filterBackup
				}
				mu.Unlock()
			}
			return nil
		})
	}

	err = g.Wait()
	r.ruleGroupMetrics.UpdateRuleGroupsInStore(ruleGroupCounts)
	return owned, backedUp, err
}

// filterRuleGroups returns map of rule groups that given instance "owns" based on supplied ring.
// This function only uses User, Namespace, and Name fields of individual RuleGroups.
//
// This method must not use r.ring, but only ring passed as parameter.
func (r *Ruler) filterRuleGroups(userID string, ruleGroups []*rulespb.RuleGroupDesc, ring ring.ReadRing) []*rulespb.RuleGroupDesc {
	// Prune the rule group to only contain rules that this ruler is responsible for, based on ring.
	var result []*rulespb.RuleGroupDesc

	for _, g := range ruleGroups {
		owned, err := r.instanceOwnsRuleGroup(ring, g, r.limits.DisabledRuleGroups(userID), false)
		if err != nil {
			switch e := err.(type) {
			case *DisabledRuleGroupErr:
				level.Info(r.logger).Log("msg", e.Message)
				continue
			default:
				r.ringCheckErrors.Inc()
				level.Error(r.logger).Log("msg", "failed to check if the ruler replica owns the rule group", "user", userID, "namespace", g.Namespace, "group", g.Name, "err", err)
				continue
			}
		}

		if owned {
			level.Debug(r.logger).Log("msg", "rule group owned", "user", g.User, "namespace", g.Namespace, "name", g.Name)
			result = append(result, g)
		} else {
			level.Debug(r.logger).Log("msg", "rule group not owned, ignoring", "user", g.User, "namespace", g.Namespace, "name", g.Name)
		}
	}

	return result
}

// filterBackupRuleGroups returns map of rule groups that given instance backs up based on supplied ring.
// This function only uses User, Namespace, and Name fields of individual RuleGroups.
//
// This method must not use r.ring, but only ring passed as parameter
func (r *Ruler) filterBackupRuleGroups(userID string, ruleGroups []*rulespb.RuleGroupDesc, owned []*rulespb.RuleGroupDesc, ring ring.ReadRing) []*rulespb.RuleGroupDesc {
	var result []*rulespb.RuleGroupDesc
	ownedMap := map[uint32]struct{}{}
	for _, g := range owned {
		hash := tokenForGroup(g)
		ownedMap[hash] = struct{}{}
	}
	for _, g := range ruleGroups {
		hash := tokenForGroup(g)
		// if already owned for eval, don't take backup ownership
		if _, OK := ownedMap[hash]; OK {
			continue
		}
		backup, err := r.instanceOwnsRuleGroup(ring, g, r.limits.DisabledRuleGroups(userID), true)
		if err != nil {
			switch e := err.(type) {
			case *DisabledRuleGroupErr:
				level.Info(r.logger).Log("msg", e.Message)
				continue
			default:
				r.ringCheckErrors.Inc()
				level.Error(r.logger).Log("msg", "failed to check if the ruler replica backs up the rule group", "user", userID, "namespace", g.Namespace, "group", g.Name, "err", err)
				continue
			}
		}

		if backup {
			level.Debug(r.logger).Log("msg", "rule group backed up", "user", g.User, "namespace", g.Namespace, "name", g.Name)
			result = append(result, g)
		} else {
			level.Debug(r.logger).Log("msg", "rule group not backed up, ignoring", "user", g.User, "namespace", g.Namespace, "name", g.Name)
		}
	}

	return result
}

// GetRules retrieves the running rules from this ruler and all running rulers in the ring if
// sharding is enabled
func (r *Ruler) GetRules(ctx context.Context, rulesRequest RulesRequest) (*RulesResponse, error) {
	userID, err := tenant.TenantID(ctx)
	if err != nil {
		return nil, fmt.Errorf("no user id found in context")
	}

	if r.cfg.EnableSharding {
		resp, err := r.getShardedRules(ctx, userID, rulesRequest)
		if resp == nil {
			return &RulesResponse{
				Groups:    make([]*GroupStateDesc, 0),
				NextToken: "",
			}, err
		}
		return resp, err
	}

	response, err := r.getLocalRules(userID, rulesRequest, false)
	return &response, err
}

func (r *Ruler) getLocalRules(userID string, rulesRequest RulesRequest, includeBackups bool) (RulesResponse, error) {
	groups := r.manager.GetRules(userID)

	groupDescs := make([]*GroupStateDesc, 0, len(groups))
	prefix := filepath.Join(r.cfg.RulePath, userID) + "/"

	sliceToSet := func(values []string) map[string]struct{} {
		set := make(map[string]struct{}, len(values))
		for _, v := range values {
			set[v] = struct{}{}
		}
		return set
	}

	ruleNameSet := sliceToSet(rulesRequest.RuleNames)
	ruleGroupNameSet := sliceToSet(rulesRequest.RuleGroupNames)
	fileSet := sliceToSet(rulesRequest.Files)
	ruleType := rulesRequest.Type
	alertState := rulesRequest.State
	health := rulesRequest.Health
	matcherSets, err := parseMatchersParam(rulesRequest.Matchers)
	if err != nil {
		return RulesResponse{}, errors.Wrap(err, "error parsing matcher values")
	}

	returnAlerts := ruleType == "" || ruleType == alertingRuleFilter
	returnRecording := (ruleType == "" || ruleType == recordingRuleFilter) && alertState == ""

	for _, group := range groups {
		// The mapped filename is url path escaped encoded to make handling `/` characters easier
		decodedNamespace, err := url.PathUnescape(strings.TrimPrefix(group.File(), prefix))
		if err != nil {
			return RulesResponse{}, errors.Wrap(err, "unable to decode rule filename")
		}
		if len(fileSet) > 0 {
			if _, OK := fileSet[decodedNamespace]; !OK {
				continue
			}
		}

		if len(ruleGroupNameSet) > 0 {
			if _, OK := ruleGroupNameSet[group.Name()]; !OK {
				continue
			}
		}
		interval := group.Interval()

		queryOffset := group.QueryOffset()
		groupDesc := &GroupStateDesc{
			Group: &rulespb.RuleGroupDesc{
				Name:        group.Name(),
				Namespace:   string(decodedNamespace),
				Interval:    interval,
				User:        userID,
				Limit:       int64(group.Limit()),
				QueryOffset: &queryOffset,
			},

			EvaluationTimestamp: group.GetLastEvaluation(),
			EvaluationDuration:  group.GetEvaluationTime(),
		}
		for _, r := range group.Rules() {
			if len(ruleNameSet) > 0 {
				if _, OK := ruleNameSet[r.Name()]; !OK {
					continue
				}
			}
			if !returnByHealth(health, string(r.Health())) {
				continue
			}
			if !matchesMatcherSets(matcherSets, r.Labels()) {
				continue
			}
			lastError := ""
			if r.LastError() != nil {
				lastError = r.LastError().Error()
			}

			var ruleDesc *RuleStateDesc
			switch rule := r.(type) {
			case *promRules.AlertingRule:
				if !returnAlerts {
					continue
				}
				if !returnByState(alertState, rule.State().String()) {
					continue
				}
				alerts := []*AlertStateDesc{}
				if !rulesRequest.ExcludeAlerts {
					for _, a := range rule.ActiveAlerts() {
						alerts = append(alerts, &AlertStateDesc{
							State:           a.State.String(),
							Labels:          cortexpb.FromLabelsToLabelAdapters(a.Labels),
							Annotations:     cortexpb.FromLabelsToLabelAdapters(a.Annotations),
							Value:           a.Value,
							ActiveAt:        a.ActiveAt,
							FiredAt:         a.FiredAt,
							ResolvedAt:      a.ResolvedAt,
							LastSentAt:      a.LastSentAt,
							ValidUntil:      a.ValidUntil,
							KeepFiringSince: a.KeepFiringSince,
						})
					}
				}
				ruleDesc = &RuleStateDesc{
					Rule: &rulespb.RuleDesc{
						Expr:          rule.Query().String(),
						Alert:         rule.Name(),
						For:           rule.HoldDuration(),
						KeepFiringFor: rule.KeepFiringFor(),
						Labels:        cortexpb.FromLabelsToLabelAdapters(rule.Labels()),
						Annotations:   cortexpb.FromLabelsToLabelAdapters(rule.Annotations()),
					},
					State:               rule.State().String(),
					Health:              string(rule.Health()),
					LastError:           lastError,
					Alerts:              alerts,
					EvaluationTimestamp: rule.GetEvaluationTimestamp(),
					EvaluationDuration:  rule.GetEvaluationDuration(),
				}
			case *promRules.RecordingRule:
				if !returnRecording {
					continue
				}
				ruleDesc = &RuleStateDesc{
					Rule: &rulespb.RuleDesc{
						Record: rule.Name(),
						Expr:   rule.Query().String(),
						Labels: cortexpb.FromLabelsToLabelAdapters(rule.Labels()),
					},
					Health:              string(rule.Health()),
					LastError:           lastError,
					EvaluationTimestamp: rule.GetEvaluationTimestamp(),
					EvaluationDuration:  rule.GetEvaluationDuration(),
				}
			default:
				return RulesResponse{}, errors.Errorf("failed to assert type of rule '%v'", rule.Name())
			}
			groupDesc.ActiveRules = append(groupDesc.ActiveRules, ruleDesc)
		}
		if len(groupDesc.ActiveRules) > 0 {
			groupDescs = append(groupDescs, groupDesc)
		}
	}

	combinedRuleStateDescs := groupDescs
	if includeBackups {
		backupGroups := r.manager.GetBackupRules(userID)
		backupGroupDescs, err := r.ruleGroupListToGroupStateDesc(userID, backupGroups, groupListFilter{
			ruleNameSet,
			ruleGroupNameSet,
			fileSet,
			returnAlerts,
			returnRecording,
			matcherSets,
		})
		if err != nil {
			return RulesResponse{}, err
		}
		combinedRuleStateDescs = append(combinedRuleStateDescs, backupGroupDescs...)
	}

	if rulesRequest.MaxRuleGroups <= 0 {
		return RulesResponse{
			Groups:    combinedRuleStateDescs,
			NextToken: "",
		}, nil
	}

	sort.Sort(PaginatedGroupStates(combinedRuleStateDescs))

	resultingGroupDescs := make([]*GroupStateDesc, 0, len(combinedRuleStateDescs))
	for _, group := range combinedRuleStateDescs {
		groupID := GetRuleGroupNextToken(group.Group.Namespace, group.Group.Name)

		// Only want groups whose groupID is greater than the token. This comparison works because
		// we sort by that groupID
		if len(rulesRequest.NextToken) > 0 && rulesRequest.NextToken >= groupID {
			continue
		}
		if len(group.ActiveRules) > 0 {
			resultingGroupDescs = append(resultingGroupDescs, group)
		}
	}

	resultingGroupDescs, nextToken := generatePage(resultingGroupDescs, int(rulesRequest.MaxRuleGroups))
	return RulesResponse{
		Groups:    resultingGroupDescs,
		NextToken: nextToken,
	}, nil
}

type groupListFilter struct {
	ruleNameSet      map[string]struct{}
	ruleGroupNameSet map[string]struct{}
	fileSet          map[string]struct{}
	returnAlerts     bool
	returnRecording  bool
	matcherSets      [][]*labels.Matcher
}

// ruleGroupListToGroupStateDesc converts rulespb.RuleGroupList to []*GroupStateDesc while accepting filters to control what goes to the
// resulting []*GroupStateDesc
func (r *Ruler) ruleGroupListToGroupStateDesc(userID string, backupGroups rulespb.RuleGroupList, filters groupListFilter) ([]*GroupStateDesc, error) {
	groupDescs := make([]*GroupStateDesc, 0, len(backupGroups))
	for _, group := range backupGroups {
		if len(filters.fileSet) > 0 {
			if _, OK := filters.fileSet[group.GetNamespace()]; !OK {
				continue
			}
		}

		if len(filters.ruleGroupNameSet) > 0 {
			if _, OK := filters.ruleGroupNameSet[group.GetName()]; !OK {
				continue
			}
		}
		interval := r.cfg.EvaluationInterval
		if group.Interval != 0 {
			interval = group.Interval
		}

		groupDesc := &GroupStateDesc{
			Group: &rulespb.RuleGroupDesc{
				Name:        group.GetName(),
				Namespace:   group.GetNamespace(),
				Interval:    interval,
				User:        userID,
				Limit:       group.Limit,
				QueryOffset: group.QueryOffset,
				Labels:      group.Labels,
			},
			// We are keeping default value for EvaluationTimestamp and EvaluationDuration since the backup is not evaluating
		}
		for _, r := range group.GetRules() {
			name := r.GetRecord()
			isAlertingRule := false
			if name == "" {
				name = r.GetAlert()
				isAlertingRule = true
			}
			if len(filters.ruleNameSet) > 0 {
				if _, OK := filters.ruleNameSet[name]; !OK {
					continue
				}
			}
			if !matchesMatcherSets(filters.matcherSets, cortexpb.FromLabelAdaptersToLabels(r.Labels)) {
				continue
			}

			var ruleDesc *RuleStateDesc
			query, err := cortexparser.ParseExpr(r.GetExpr())
			if err != nil {
				return nil, errors.Errorf("failed to parse rule query '%v'", r.GetExpr())
			}
			if isAlertingRule {
				if !filters.returnAlerts {
					continue
				}
				alerts := []*AlertStateDesc{} // backup rules are not evaluated so there will be no active alerts
				ruleDesc = &RuleStateDesc{
					Rule: &rulespb.RuleDesc{
						Expr:          query.String(),
						Alert:         name,
						For:           r.GetFor(),
						KeepFiringFor: r.GetKeepFiringFor(),
						Labels:        r.Labels,
						Annotations:   r.Annotations,
					},
					State:               promRules.StateInactive.String(), // backup rules are not evaluated so they are inactive
					Health:              string(promRules.HealthUnknown),
					Alerts:              alerts,
					EvaluationTimestamp: time.Time{},
					EvaluationDuration:  time.Duration(0),
				}
			} else {
				if !filters.returnRecording {
					continue
				}
				ruleDesc = &RuleStateDesc{
					Rule: &rulespb.RuleDesc{
						Record: name,
						Expr:   query.String(),
						Labels: r.Labels,
					},
					Health:              string(promRules.HealthUnknown),
					EvaluationTimestamp: time.Time{},
					EvaluationDuration:  time.Duration(0),
				}
			}
			groupDesc.ActiveRules = append(groupDesc.ActiveRules, ruleDesc)
		}
		if len(groupDesc.ActiveRules) > 0 {
			groupDescs = append(groupDescs, groupDesc)
		}
	}
	return groupDescs, nil
}

func (r *Ruler) getShardSizeForUser(userID string) int {
	newShardSize := util.DynamicShardSize(r.limits.RulerTenantShardSize(userID), r.ring.InstancesCount())

	// We want to guarantee that shard size will be at least replication factor
	return max(newShardSize, r.cfg.Ring.ReplicationFactor)
}

func (r *Ruler) getShardedRules(ctx context.Context, userID string, rulesRequest RulesRequest) (*RulesResponse, error) {
	ring := ring.ReadRing(r.ring)

	if shardSize := r.limits.RulerTenantShardSize(userID); shardSize > 0 && r.cfg.ShardingStrategy == util.ShardingStrategyShuffle {
		ring = r.ring.ShuffleShard(userID, r.getShardSizeForUser(userID))
	}

	rulers, failedZones, err := GetReplicationSetForListRule(ring, &r.cfg.Ring)
	if err != nil {
		return nil, err
	}

	ctx, err = user.InjectIntoGRPCRequest(ctx)
	if err != nil {
		return nil, fmt.Errorf("unable to inject user ID into grpc request, %v", err)
	}

	var (
		mtx      sync.Mutex
		merged   []*RulesResponse
		errCount int
	)

	zoneByAddress := make(map[string]string)
	if r.cfg.RulesBackupEnabled() {
		for _, ruler := range rulers.Instances {
			zoneByAddress[ruler.Addr] = ruler.Zone
		}
	}
	// Concurrently fetch rules from all rulers.
	jobs := concurrency.CreateJobsFromStrings(rulers.GetAddresses())
	err = concurrency.ForEach(ctx, jobs, len(jobs), func(ctx context.Context, job interface{}) error {
		addr := job.(string)

		rulerClient, err := r.clientsPool.GetClientFor(addr)
		if err != nil {
			return errors.Wrapf(err, "unable to get client for ruler %s", addr)
		}

		ctx, cancel := context.WithTimeout(ctx, r.cfg.ClientTLSConfig.RemoteTimeout)
		defer cancel()
		newGrps, err := rulerClient.Rules(ctx, &RulesRequest{
			RuleNames:      rulesRequest.GetRuleNames(),
			RuleGroupNames: rulesRequest.GetRuleGroupNames(),
			Files:          rulesRequest.GetFiles(),
			Type:           rulesRequest.GetType(),
			State:          rulesRequest.GetState(),
			Health:         rulesRequest.GetHealth(),
			Matchers:       rulesRequest.GetMatchers(),
			ExcludeAlerts:  rulesRequest.GetExcludeAlerts(),
			MaxRuleGroups:  rulesRequest.GetMaxRuleGroups(),
			NextToken:      rulesRequest.GetNextToken(),
		})

		if err != nil {
			level.Error(r.logger).Log("msg", "unable to retrieve rules from ruler", "addr", addr, "err", err)
			r.rulerGetRulesFailures.WithLabelValues(addr).Inc()
			// If rules backup is enabled and there are enough rulers replicating the rules, we should
			// be able to handle failures.
			if r.cfg.RulesBackupEnabled() && len(jobs) >= r.cfg.Ring.ReplicationFactor {
				mtx.Lock()
				failedZones[zoneByAddress[addr]] = struct{}{}
				errCount += 1
				failed := (rulers.MaxUnavailableZones > 0 && len(failedZones) > rulers.MaxUnavailableZones) || (rulers.MaxUnavailableZones <= 0 && errCount > rulers.MaxErrors)
				mtx.Unlock()
				if !failed {
					return nil
				}
			}
			return errors.Wrapf(err, "unable to retrieve rules from ruler %s", addr)
		}

		mtx.Lock()
		merged = append(merged, newGrps)
		mtx.Unlock()

		return nil
	})

	if err == nil {
		if r.cfg.RulesBackupEnabled() || r.cfg.APIDeduplicateRules {
			return mergeGroupStateDesc(merged, rulesRequest.MaxRuleGroups, true), nil
		}
		return mergeGroupStateDesc(merged, rulesRequest.MaxRuleGroups, false), nil
	}

	return &RulesResponse{
		Groups:    make([]*GroupStateDesc, 0),
		NextToken: "",
	}, err
}

// Rules implements the rules service
func (r *Ruler) Rules(ctx context.Context, in *RulesRequest) (*RulesResponse, error) {
	userID, err := tenant.TenantID(ctx)

	if err != nil {
		return nil, fmt.Errorf("no user id found in context")
	}

	response, err := r.getLocalRules(userID, *in, r.cfg.RulesBackupEnabled())
	if err != nil {
		return nil, err
	}

	return &response, nil
}

// HasMaxRuleGroupsLimit check if RulerMaxRuleGroupsPerTenant limit is set for the userID.
func (r *Ruler) HasMaxRuleGroupsLimit(userID string) bool {
	limit := r.limits.RulerMaxRuleGroupsPerTenant(userID)
	return limit > 0
}

// AssertMaxRuleGroups limit has not been reached compared to the current
// number of total rule groups in input and returns an error if so.
func (r *Ruler) AssertMaxRuleGroups(userID string, rg int) error {
	limit := r.limits.RulerMaxRuleGroupsPerTenant(userID)

	if limit <= 0 {
		return nil
	}

	if rg <= limit {
		return nil
	}

	return fmt.Errorf(errMaxRuleGroupsPerUserLimitExceeded, limit, rg)
}

// AssertMaxRulesPerRuleGroup limit has not been reached compared to the current
// number of rules in a rule group in input and returns an error if so.
func (r *Ruler) AssertMaxRulesPerRuleGroup(userID string, rules int) error {
	limit := r.limits.RulerMaxRulesPerRuleGroup(userID)

	if limit <= 0 {
		return nil
	}

	if rules <= limit {
		return nil
	}
	return fmt.Errorf(errMaxRulesPerRuleGroupPerUserLimitExceeded, limit, rules)
}

func (r *Ruler) DeleteTenantConfiguration(w http.ResponseWriter, req *http.Request) {
	logger := util_log.WithContext(req.Context(), r.logger)

	userID, err := tenant.TenantID(req.Context())
	if err != nil {
		// When Cortex is running, it uses Auth Middleware for checking X-Scope-OrgID and injecting tenant into context.
		// Auth Middleware sends http.StatusUnauthorized if X-Scope-OrgID is missing, so we do too here, for consistency.
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	err = r.store.DeleteNamespace(req.Context(), userID, "") // Empty namespace = delete all rule groups.
	if err != nil && !errors.Is(err, rulestore.ErrGroupNamespaceNotFound) {
		util_api.RespondError(logger, w, v1.ErrServer, err.Error(), http.StatusInternalServerError)
		return
	}

	level.Info(logger).Log("msg", "deleted all tenant rule groups", "user", userID)
	w.WriteHeader(http.StatusOK)
}

func (r *Ruler) ListAllRules(w http.ResponseWriter, req *http.Request) {
	logger := util_log.WithContext(req.Context(), r.logger)

	userIDs, err := r.store.ListAllUsers(req.Context())
	if err != nil {
		level.Error(logger).Log("msg", errListAllUser, "err", err)
		http.Error(w, fmt.Sprintf("%s: %s", errListAllUser, err.Error()), http.StatusInternalServerError)
		return
	}

	done := make(chan struct{})
	iter := make(chan interface{})

	go func() {
		util.StreamWriteYAMLV3Response(w, iter, logger)
		close(done)
	}()

	err = concurrency.ForEachUser(req.Context(), userIDs, fetchRulesConcurrency, func(ctx context.Context, userID string) error {
		rg, err := r.store.ListRuleGroupsForUserAndNamespace(ctx, userID, "")
		if err != nil {
			return errors.Wrapf(err, "failed to fetch ruler config for user %s", userID)
		}
		userRules := map[string]rulespb.RuleGroupList{userID: rg}
		if userRules, err = r.store.LoadRuleGroups(ctx, userRules); err != nil {
			return errors.Wrapf(err, "failed to load ruler config for user %s", userID)
		}
		data := map[string]map[string][]rulefmt.RuleGroup{userID: userRules[userID].Formatted()}

		select {
		case iter <- data:
		case <-done: // stop early, if sending response has already finished
		}

		return nil
	})
	if err != nil {
		level.Error(logger).Log("msg", "failed to list all ruler configs", "err", err)
	}
	close(iter)
	<-done
}

func returnByState(requestState string, alertState string) bool {
	return requestState == "" || requestState == alertState
}

func returnByHealth(requestHealth string, ruleHealth string) bool {
	return requestHealth == "" || requestHealth == ruleHealth
}

func parseMatchersParam(matchers []string) ([][]*labels.Matcher, error) {
	var matcherSets [][]*labels.Matcher
	for _, s := range matchers {
		matchers, err := parser.ParseMetricSelector(s)
		if err != nil {
			return nil, err
		}
		matcherSets = append(matcherSets, matchers)
	}

OUTER:
	for _, ms := range matcherSets {
		for _, lm := range ms {
			if lm != nil && !lm.Matches("") {
				continue OUTER
			}
		}
		return nil, errors.New("match[] must contain at least one non-empty matcher")
	}
	return matcherSets, nil
}

func matches(l labels.Labels, matchers ...*labels.Matcher) bool {
	for _, m := range matchers {
		if v := l.Get(m.Name); !m.Matches(v) {
			return false
		}
	}
	return true
}

// matchesMatcherSets ensures all matches in each matcher set are ANDed and the set of those is ORed.
func matchesMatcherSets(matcherSets [][]*labels.Matcher, l labels.Labels) bool {
	if len(matcherSets) == 0 {
		return true
	}

	for _, matchers := range matcherSets {
		if matches(l, matchers...) {
			return true
		}
	}
	return false
}
