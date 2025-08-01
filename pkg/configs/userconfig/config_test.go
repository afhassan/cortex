package userconfig

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/rulefmt"
	"github.com/prometheus/prometheus/rules"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/cortexproject/cortex/pkg/parser"
	util_log "github.com/cortexproject/cortex/pkg/util/log"
)

var legacyRulesFile = `ALERT TestAlert
IF up == 0
FOR 5m
LABELS { severity = "critical" }
ANNOTATIONS {
	message = "I am a message"
}`

var ruleFile = `groups:
- name: example
  rules:
  - alert: TestAlert
    expr: up == 0
    for: 5m
    labels:
      severity: critical
    annotations:
      message: I am a message`

func TestUnmarshalJSONLegacyConfigWithMissingRuleFormatVersionSucceeds(t *testing.T) {
	actual := Config{}
	buf := []byte(`{"rules_files": {"a": "b"}}`)
	assert.Nil(t, json.Unmarshal(buf, &actual))

	expected := Config{
		RulesConfig: RulesConfig{
			Files: map[string]string{
				"a": "b",
			},
			FormatVersion: RuleFormatV1,
		},
	}

	assert.Equal(t, expected, actual)
}

func TestUnmarshalYAMLLegacyConfigWithMissingRuleFormatVersionSucceeds(t *testing.T) {
	actual := Config{}
	buf := []byte(strings.TrimSpace(`
rule_format_version: '1'
rules_files:
  a: b
`))
	assert.Nil(t, yaml.Unmarshal(buf, &actual))

	expected := Config{
		RulesConfig: RulesConfig{
			Files: map[string]string{
				"a": "b",
			},
			FormatVersion: RuleFormatV1,
		},
	}

	assert.Equal(t, expected, actual)
}

func TestParseLegacyAlerts(t *testing.T) {
	parsed, err := parser.ParseExpr("up == 0")
	require.NoError(t, err)
	rule := rules.NewAlertingRule(
		"TestAlert",
		parsed,
		5*time.Minute,
		0,
		labels.FromStrings("severity", "critical"),
		labels.FromStrings("message", "I am a message"),
		labels.EmptyLabels(),
		"",
		true,
		util_log.GoKitLogToSlog(log.With(util_log.Logger, "alert", "TestAlert")),
	)

	for i, tc := range []struct {
		cfg      RulesConfig
		expected map[string][]rules.Rule
		err      error
	}{
		{
			cfg: RulesConfig{
				FormatVersion: RuleFormatV1,
			},
			err: fmt.Errorf("unsupported rule format version 0"),
		},
		{
			cfg: RulesConfig{
				FormatVersion: RuleFormatV2,
				Files: map[string]string{
					"alerts.yaml": `
groups:
- name: example
  rules:
  - alert: TestAlert
    expr: up == 0
    for: 5m
    labels:
      severity: critical
    annotations:
      message: I am a message
`,
				},
			},
			expected: map[string][]rules.Rule{
				"example;alerts.yaml": {rule},
			},
		},
	} {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			rules, err := tc.cfg.Parse()
			if tc.err != nil {
				require.Equal(t, err, tc.err)
			} else {
				require.NoError(t, err)
				require.Equal(t, 1, len(rules["example;alerts.yaml"]))
				require.Equal(t, tc.expected["example;alerts.yaml"][0].String(), rules["example;alerts.yaml"][0].String())
			}
		})
	}
}

func TestParseFormatted(t *testing.T) {
	dur, err := model.ParseDuration("5m")
	require.NoError(t, err)

	rulesV2 := []rulefmt.Rule{
		{
			Alert: "TestAlert",
			Expr:  "up == 0",
			For:   dur,
			Labels: map[string]string{
				"severity": "critical",
			},
			Annotations: map[string]string{
				"message": "I am a message",
			},
		},
	}

	for i, tc := range []struct {
		cfg      RulesConfig
		expected map[string]rulefmt.RuleGroups
		err      error
	}{
		{
			cfg: RulesConfig{
				FormatVersion: RuleFormatV1,
				Files: map[string]string{
					"legacy.rules": legacyRulesFile,
				},
			},
			err: fmt.Errorf("unsupported rule format version 0"),
		},
		{
			cfg: RulesConfig{
				FormatVersion: RuleFormatV2,
				Files: map[string]string{
					"alerts.yaml": ruleFile,
				},
			},
			expected: map[string]rulefmt.RuleGroups{
				"alerts.yaml": {
					Groups: []rulefmt.RuleGroup{
						{
							Name:  "example",
							Rules: rulesV2,
						},
					},
				},
			},
		},
	} {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			rules, err := tc.cfg.ParseFormatted()
			if tc.err != nil {
				require.Equal(t, err, tc.err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.expected, rules)
			}
		})
	}
}
