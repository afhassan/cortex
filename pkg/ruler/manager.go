package ruler

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	ot "github.com/opentracing/opentracing-go"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/discovery"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/rulefmt"
	"github.com/prometheus/prometheus/notifier"
	promRules "github.com/prometheus/prometheus/rules"
	"github.com/weaveworks/common/user"
	"golang.org/x/net/context/ctxhttp"

	"github.com/cortexproject/cortex/pkg/ring/client"
	"github.com/cortexproject/cortex/pkg/ruler/rulespb"
)

type DefaultMultiTenantManager struct {
	cfg             Config
	notifierCfg     *config.Config
	managerFactory  ManagerFactory
	ruleEvalMetrics *RuleEvalMetrics
	frontendPool    *client.Pool

	mapper *mapper

	// Structs for holding per-user Prometheus rules Managers
	// and a corresponding metrics struct
	userManagerMtx     sync.RWMutex
	userManagers       map[string]RulesManager
	userManagerMetrics *ManagerMetrics

	// Per-user notifiers with separate queues.
	notifiersMtx              sync.Mutex
	notifiers                 map[string]*rulerNotifier
	notifiersDiscoveryMetrics map[string]discovery.DiscovererMetrics

	// Per-user externalLabels.
	userExternalLabels *userExternalLabels

	// rules backup
	rulesBackupManager *rulesBackupManager

	managersTotal                 prometheus.Gauge
	lastReloadSuccessful          *prometheus.GaugeVec
	lastReloadSuccessfulTimestamp *prometheus.GaugeVec
	configUpdatesTotal            *prometheus.CounterVec
	registry                      prometheus.Registerer
	logger                        log.Logger

	ruleCache    map[string][]*promRules.Group
	ruleCacheMtx sync.RWMutex
	syncRuleMtx  sync.Mutex

	ruleGroupIterationFunc promRules.GroupEvalIterationFunc
}

func NewDefaultMultiTenantManager(cfg Config, limits RulesLimits, managerFactory ManagerFactory, evalMetrics *RuleEvalMetrics, reg prometheus.Registerer, logger log.Logger) (*DefaultMultiTenantManager, error) {
	ncfg, err := buildNotifierConfig(&cfg)
	if err != nil {
		return nil, err
	}

	userManagerMetrics := NewManagerMetrics(cfg.DisableRuleGroupLabel)
	if reg != nil {
		reg.MustRegister(userManagerMetrics)
	}

	discoveryMetricRegister := reg
	if discoveryMetricRegister == nil {
		discoveryMetricRegister = prometheus.DefaultRegisterer
	}

	notifiersDiscoveryMetrics, err := discovery.CreateAndRegisterSDMetrics(discoveryMetricRegister)

	if err != nil {
		level.Error(logger).Log("msg", "failed to register service discovery metrics", "err", err)
		os.Exit(1)
	}

	m := &DefaultMultiTenantManager{
		cfg:                       cfg,
		notifierCfg:               ncfg,
		managerFactory:            managerFactory,
		frontendPool:              newFrontendPool(cfg, logger, reg),
		ruleEvalMetrics:           evalMetrics,
		notifiers:                 map[string]*rulerNotifier{},
		userExternalLabels:        newUserExternalLabels(cfg.ExternalLabels, limits),
		notifiersDiscoveryMetrics: notifiersDiscoveryMetrics,
		mapper:                    newMapper(cfg.RulePath, logger),
		userManagers:              map[string]RulesManager{},
		userManagerMetrics:        userManagerMetrics,
		ruleCache:                 map[string][]*promRules.Group{},
		managersTotal: promauto.With(reg).NewGauge(prometheus.GaugeOpts{
			Namespace: "cortex",
			Name:      "ruler_managers_total",
			Help:      "Total number of managers registered and running in the ruler",
		}),
		lastReloadSuccessful: promauto.With(reg).NewGaugeVec(prometheus.GaugeOpts{
			Namespace: "cortex",
			Name:      "ruler_config_last_reload_successful",
			Help:      "Boolean set to 1 whenever the last configuration reload attempt was successful.",
		}, []string{"user"}),
		lastReloadSuccessfulTimestamp: promauto.With(reg).NewGaugeVec(prometheus.GaugeOpts{
			Namespace: "cortex",
			Name:      "ruler_config_last_reload_successful_seconds",
			Help:      "Timestamp of the last successful configuration reload.",
		}, []string{"user"}),
		configUpdatesTotal: promauto.With(reg).NewCounterVec(prometheus.CounterOpts{
			Namespace: "cortex",
			Name:      "ruler_config_updates_total",
			Help:      "Total number of config updates triggered by a user",
		}, []string{"user"}),
		registry:               reg,
		logger:                 logger,
		ruleGroupIterationFunc: defaultRuleGroupIterationFunc,
	}
	if cfg.RulesBackupEnabled() {
		m.rulesBackupManager = newRulesBackupManager(cfg, logger, reg)
	}
	return m, nil
}

func NewDefaultMultiTenantManagerWithIterationFunc(iterFunc promRules.GroupEvalIterationFunc, cfg Config, limits RulesLimits, managerFactory ManagerFactory, evalMetrics *RuleEvalMetrics, reg prometheus.Registerer, logger log.Logger) (*DefaultMultiTenantManager, error) {
	manager, err := NewDefaultMultiTenantManager(cfg, limits, managerFactory, evalMetrics, reg, logger)
	if err != nil {
		return nil, err
	}
	manager.ruleGroupIterationFunc = iterFunc
	return manager, nil
}

func (r *DefaultMultiTenantManager) SyncRuleGroups(ctx context.Context, ruleGroups map[string]rulespb.RuleGroupList) {
	// this is a safety lock to ensure this method is executed sequentially
	r.syncRuleMtx.Lock()
	defer r.syncRuleMtx.Unlock()

	for userID, ruleGroup := range ruleGroups {
		r.syncRulesToManager(ctx, userID, ruleGroup)
	}

	r.userManagerMtx.Lock()
	defer r.userManagerMtx.Unlock()

	// Check for deleted users and remove them
	for userID, mngr := range r.userManagers {
		if _, exists := ruleGroups[userID]; !exists {
			go mngr.Stop()
			delete(r.userManagers, userID)

			r.removeNotifier(userID)
			r.mapper.cleanupUser(userID)
			r.userExternalLabels.remove(userID)
			r.lastReloadSuccessful.DeleteLabelValues(userID)
			r.lastReloadSuccessfulTimestamp.DeleteLabelValues(userID)
			r.configUpdatesTotal.DeleteLabelValues(userID)
			r.userManagerMetrics.RemoveUserRegistry(userID)
			if r.ruleEvalMetrics != nil {
				r.ruleEvalMetrics.deletePerUserMetrics(userID)
			}
			level.Info(r.logger).Log("msg", "deleted rule manager and local rule files", "user", userID)
		}
	}

	r.managersTotal.Set(float64(len(r.userManagers)))
}

func (r *DefaultMultiTenantManager) updateRuleCache(user string, rules []*promRules.Group) {
	r.ruleCacheMtx.Lock()
	defer r.ruleCacheMtx.Unlock()
	r.ruleCache[user] = rules
}

func (r *DefaultMultiTenantManager) deleteRuleCache(user string) {
	r.ruleCacheMtx.Lock()
	defer r.ruleCacheMtx.Unlock()
	delete(r.ruleCache, user)
}

func (r *DefaultMultiTenantManager) BackUpRuleGroups(ctx context.Context, ruleGroups map[string]rulespb.RuleGroupList) {
	if r.rulesBackupManager != nil {
		r.rulesBackupManager.setRuleGroups(ctx, ruleGroups)
	}
}

// syncRulesToManager maps the rule files to disk, detects any changes and will create/update the
// users Prometheus Rules Manager.
func (r *DefaultMultiTenantManager) syncRulesToManager(ctx context.Context, user string, groups rulespb.RuleGroupList) {
	// Map the files to disk and return the file names to be passed to the users manager if they
	// have been updated
	rulesUpdated, files, err := r.mapper.MapRules(user, groups.Formatted())
	if err != nil {
		r.lastReloadSuccessful.WithLabelValues(user).Set(0)
		level.Error(r.logger).Log("msg", "unable to map rule files", "user", user, "err", err)
		return
	}
	externalLabels, externalLabelsUpdated := r.userExternalLabels.update(user)

	existing := true
	manager := r.getRulesManager(user, ctx)
	if manager == nil {
		existing = false
		manager = r.createRulesManager(user, ctx)
	}

	if manager == nil {
		return
	}

	if !existing || rulesUpdated || externalLabelsUpdated {
		level.Debug(r.logger).Log("msg", "updating rules", "user", user)
		r.configUpdatesTotal.WithLabelValues(user).Inc()
		if (rulesUpdated || externalLabelsUpdated) && existing {
			r.updateRuleCache(user, manager.RuleGroups())
		}
		err = manager.Update(r.cfg.EvaluationInterval, files, externalLabels, r.cfg.ExternalURL.String(), r.ruleGroupIterationFunc)
		r.deleteRuleCache(user)
		if err != nil {
			r.lastReloadSuccessful.WithLabelValues(user).Set(0)
			level.Error(r.logger).Log("msg", "unable to update rule manager", "user", user, "err", err)
			return
		}
		if externalLabelsUpdated {
			if err = r.notifierApplyExternalLabels(user, externalLabels); err != nil {
				r.lastReloadSuccessful.WithLabelValues(user).Set(0)
				level.Error(r.logger).Log("msg", "unable to update notifier", "user", user, "err", err)
				return
			}
		}

		r.lastReloadSuccessful.WithLabelValues(user).Set(1)
		r.lastReloadSuccessfulTimestamp.WithLabelValues(user).SetToCurrentTime()
	}
}

func (r *DefaultMultiTenantManager) getRulesManager(user string, ctx context.Context) RulesManager {
	r.userManagerMtx.RLock()
	defer r.userManagerMtx.RUnlock()
	return r.userManagers[user]
}

func (r *DefaultMultiTenantManager) createRulesManager(user string, ctx context.Context) RulesManager {
	r.userManagerMtx.Lock()
	defer r.userManagerMtx.Unlock()

	manager, err := r.newManager(ctx, user)
	if err != nil {
		r.lastReloadSuccessful.WithLabelValues(user).Set(0)
		level.Error(r.logger).Log("msg", "unable to create rule manager", "user", user, "err", err)
		return nil
	}
	// manager.Run() starts running the manager and blocks until Stop() is called.
	// Hence run it as another goroutine.
	go manager.Run()
	r.userManagers[user] = manager
	return manager
}

func defaultRuleGroupIterationFunc(ctx context.Context, g *promRules.Group, evalTimestamp time.Time) {
	logMessage := []interface{}{
		"component", "ruler",
		"rule_group", g.Name(),
		"namespace", g.File(),
		"num_rules", len(g.Rules()),
		"num_alert_rules", len(g.AlertingRules()),
		"eval_interval", g.Interval(),
		"eval_time", evalTimestamp,
	}

	g.Logger().Info("evaluating rule group", logMessage...)
	promRules.DefaultEvalIterationFunc(ctx, g, evalTimestamp)
}

// newManager creates a prometheus rule manager wrapped with a user id
// configured storage, appendable, notifier, and instrumentation
func (r *DefaultMultiTenantManager) newManager(ctx context.Context, userID string) (RulesManager, error) {
	// Create a new Prometheus registry and register it within
	// our metrics struct for the provided user if it doesn't already exist.
	reg := prometheus.NewRegistry()
	r.userManagerMetrics.AddUserRegistry(userID, reg)

	notifier, err := r.getOrCreateNotifier(userID, reg)
	if err != nil {
		return nil, err
	}

	return r.managerFactory(ctx, userID, notifier, r.logger, r.frontendPool, reg)
}

func (r *DefaultMultiTenantManager) removeNotifier(userID string) {
	r.notifiersMtx.Lock()
	defer r.notifiersMtx.Unlock()

	if n, ok := r.notifiers[userID]; ok {
		n.stop()
	}

	delete(r.notifiers, userID)
}

func (r *DefaultMultiTenantManager) getOrCreateNotifier(userID string, userManagerRegistry prometheus.Registerer) (*notifier.Manager, error) {
	r.notifiersMtx.Lock()
	defer r.notifiersMtx.Unlock()

	n, ok := r.notifiers[userID]
	if ok {
		// When there is a stale user, we stop the notifier but do not remove it
		n.run()
		return n.notifier, nil
	}

	logger := log.With(r.logger, "user", userID)

	n = newRulerNotifier(&notifier.Options{
		QueueCapacity: r.cfg.NotificationQueueCapacity,
		Registerer:    userManagerRegistry,
		Do: func(ctx context.Context, client *http.Client, req *http.Request) (*http.Response, error) {
			// Note: The passed-in context comes from the Prometheus notifier
			// and does *not* contain the userID. So it needs to be added to the context
			// here before using the context to inject the userID into the HTTP request.
			ctx = user.InjectOrgID(ctx, userID)
			if err := user.InjectOrgIDIntoHTTPRequest(ctx, req); err != nil {
				return nil, err
			}
			// Jaeger complains the passed-in context has an invalid span ID, so start a new root span
			sp := ot.GlobalTracer().StartSpan("notify", ot.Tag{Key: "organization", Value: userID})
			defer sp.Finish()
			ctx = ot.ContextWithSpan(ctx, sp)
			_ = ot.GlobalTracer().Inject(sp.Context(), ot.HTTPHeaders, ot.HTTPHeadersCarrier(req.Header))
			resp, err := ctxhttp.Do(ctx, client, req)
			if err != nil {
				level.Error(logger).Log("msg", "error occurred while sending alerts", "error", err)
				return resp, err
			}
			defer resp.Body.Close()
			if resp.StatusCode/100 != 2 {
				bodyBytes, err := io.ReadAll(resp.Body)
				if err != nil {
					level.Error(logger).Log("msg", "error reading response body", "error", err, "response code", resp.StatusCode)
					return resp, err
				}
				customErrorMessage := string(bodyBytes)
				if len(customErrorMessage) >= 150 {
					customErrorMessage = customErrorMessage[:150]
				}
				level.Error(logger).Log("msg", "error occurred sending notification", "error", customErrorMessage, "response code", resp.StatusCode)
			}
			return resp, err
		},
	}, logger, userManagerRegistry, r.notifiersDiscoveryMetrics)

	n.run()

	// This should never fail, unless there's a programming mistake.
	if err := n.applyConfig(r.notifierCfg); err != nil {
		return nil, err
	}

	r.notifiers[userID] = n
	return n.notifier, nil
}

func (r *DefaultMultiTenantManager) notifierApplyExternalLabels(userID string, externalLabels labels.Labels) error {
	r.notifiersMtx.Lock()
	defer r.notifiersMtx.Unlock()

	n, ok := r.notifiers[userID]
	if !ok {
		return fmt.Errorf("notifier not found")
	}
	cfg := *r.notifierCfg // Copy it
	cfg.GlobalConfig.ExternalLabels = externalLabels
	return n.applyConfig(&cfg)
}

func (r *DefaultMultiTenantManager) getCachedRules(userID string) ([]*promRules.Group, bool) {
	r.ruleCacheMtx.RLock()
	defer r.ruleCacheMtx.RUnlock()
	groups, exists := r.ruleCache[userID]
	return groups, exists
}

func (r *DefaultMultiTenantManager) GetRules(userID string) []*promRules.Group {
	var groups []*promRules.Group
	groups, cached := r.getCachedRules(userID)
	if cached {
		return groups
	}
	r.userManagerMtx.RLock()
	mngr, exists := r.userManagers[userID]
	r.userManagerMtx.RUnlock()
	if exists {
		groups = mngr.RuleGroups()
	}
	return groups
}

func (r *DefaultMultiTenantManager) GetBackupRules(userID string) rulespb.RuleGroupList {
	if r.rulesBackupManager != nil {
		return r.rulesBackupManager.getRuleGroups(userID)
	}
	return nil
}

func (r *DefaultMultiTenantManager) Stop() {
	r.notifiersMtx.Lock()
	for _, n := range r.notifiers {
		n.stop()
	}
	r.notifiersMtx.Unlock()

	level.Info(r.logger).Log("msg", "stopping user managers")
	wg := sync.WaitGroup{}
	r.userManagerMtx.Lock()
	for user, manager := range r.userManagers {
		level.Debug(r.logger).Log("msg", "shutting down user  manager", "user", user)
		wg.Add(1)
		go func(manager RulesManager, user string) {
			manager.Stop()
			wg.Done()
			level.Debug(r.logger).Log("msg", "user manager shut down", "user", user)
		}(manager, user)
	}
	wg.Wait()
	r.userManagerMtx.Unlock()
	level.Info(r.logger).Log("msg", "all user managers stopped")

	// cleanup user rules directories
	r.mapper.cleanup()
	r.userExternalLabels.cleanup()
}

func (*DefaultMultiTenantManager) ValidateRuleGroup(g rulefmt.RuleGroup) []error {
	var errs []error

	if g.Name == "" {
		errs = append(errs, errors.New("invalid rules config: rule group name must not be empty"))
		return errs
	}

	if len(g.Rules) == 0 {
		errs = append(errs, fmt.Errorf("invalid rules config: rule group '%s' has no rules", g.Name))
		return errs
	}

	for i, r := range g.Rules {
		for _, err := range r.Validate(rulefmt.RuleNode{}) {
			var ruleName string
			if r.Alert != "" {
				ruleName = r.Alert
			} else {
				ruleName = r.Record
			}
			errs = append(errs, &rulefmt.Error{
				Group:    g.Name,
				Rule:     i,
				RuleName: ruleName,
				Err:      err,
			})
		}
	}

	return errs
}
