package ruler

import (
	"net/url"
	"os"
	"testing"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus/prometheus/model/rulefmt"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"
)

var (
	testUser = "user1"

	fileOneEncoded = url.PathEscape("file /one")
	fileTwoEncoded = url.PathEscape("file /two")

	fileOnePath = "/rules/user1/" + fileOneEncoded
	fileTwoPath = "/rules/user1/" + fileTwoEncoded

	specialCharFile        = "+A_/ReallyStrange<>NAME:SPACE/?"
	specialCharFileEncoded = url.PathEscape(specialCharFile)
	specialCharFilePath    = "/rules/user1/" + specialCharFileEncoded

	initialRuleSet           map[string][]rulefmt.RuleGroup
	outOfOrderRuleSet        map[string][]rulefmt.RuleGroup
	updatedRuleSet           map[string][]rulefmt.RuleGroup
	twoFilesRuleSet          map[string][]rulefmt.RuleGroup
	twoFilesUpdatedRuleSet   map[string][]rulefmt.RuleGroup
	twoFilesDeletedRuleSet   map[string][]rulefmt.RuleGroup
	specialCharactersRuleSet map[string][]rulefmt.RuleGroup
)

func setupRuleSets() {
	initialRuleSet = map[string][]rulefmt.RuleGroup{
		"file /one": {
			{
				Name: "rulegroup_one",
				Rules: []rulefmt.Rule{
					{
						Record: "example_rule",
						Expr:   "example_expr",
					},
				},
			},
			{
				Name: "rulegroup_two",
				Rules: []rulefmt.Rule{
					{
						Record: "example_rule",
						Expr:   "example_expr",
					},
				},
			},
		},
	}
	outOfOrderRuleSet = map[string][]rulefmt.RuleGroup{
		"file /one": {
			{
				Name: "rulegroup_two",
				Rules: []rulefmt.Rule{
					{
						Record: "example_rule",
						Expr:   "example_expr",
					},
				},
			},
			{
				Name: "rulegroup_one",
				Rules: []rulefmt.Rule{
					{
						Record: "example_rule",
						Expr:   "example_expr",
					},
				},
			},
		},
	}
	updatedRuleSet = map[string][]rulefmt.RuleGroup{
		"file /one": {
			{
				Name: "rulegroup_one",
				Rules: []rulefmt.Rule{
					{
						Record: "example_rule",
						Expr:   "example_expr",
					},
				},
			},
			{
				Name: "rulegroup_two",
				Rules: []rulefmt.Rule{
					{
						Record: "example_rule",
						Expr:   "example_expr",
					},
				},
			},
			{
				Name: "rulegroup_three",
				Rules: []rulefmt.Rule{
					{
						Record: "example_rule",
						Expr:   "example_expr",
					},
				},
			},
		},
	}
	twoFilesRuleSet = map[string][]rulefmt.RuleGroup{
		"file /one": {
			{
				Name: "rulegroup_one",
				Rules: []rulefmt.Rule{
					{
						Record: "example_rule",
						Expr:   "example_expr",
					},
				},
			},
			{
				Name: "rulegroup_two",
				Rules: []rulefmt.Rule{
					{
						Record: "example_rule",
						Expr:   "example_expr",
					},
				},
			},
		},
		"file /two": {
			{
				Name: "rulegroup_one",
				Rules: []rulefmt.Rule{
					{
						Record: "example_rule",
						Expr:   "example_expr",
					},
				},
			},
		},
	}
	twoFilesUpdatedRuleSet = map[string][]rulefmt.RuleGroup{
		"file /one": {
			{
				Name: "rulegroup_one",
				Rules: []rulefmt.Rule{
					{
						Record: "example_rule",
						Expr:   "example_expr",
					},
				},
			},
			{
				Name: "rulegroup_two",
				Rules: []rulefmt.Rule{
					{
						Record: "example_rule",
						Expr:   "example_expr",
					},
				},
			},
		},
		"file /two": {
			{
				Name: "rulegroup_one",
				Rules: []rulefmt.Rule{
					{
						Record: "example_ruleupdated",
						Expr:   "example_exprupdated",
					},
				},
			},
		},
	}
	twoFilesDeletedRuleSet = map[string][]rulefmt.RuleGroup{
		"file /one": {
			{
				Name: "rulegroup_one",
				Rules: []rulefmt.Rule{
					{
						Record: "example_rule",
						Expr:   "example_expr",
					},
				},
			},
			{
				Name: "rulegroup_two",
				Rules: []rulefmt.Rule{
					{
						Record: "example_rule",
						Expr:   "example_expr",
					},
				},
			},
		},
	}
	specialCharactersRuleSet = map[string][]rulefmt.RuleGroup{
		specialCharFile: {
			{
				Name: "rulegroup_one",
				Rules: []rulefmt.Rule{
					{
						Record: "example_rule",
						Expr:   "example_expr",
					},
				},
			},
		},
	}
}

func Test_mapper_MapRules(t *testing.T) {
	l := log.NewLogfmtLogger(os.Stdout)
	l = level.NewFilter(l, level.AllowInfo())
	setupRuleSets()
	m := &mapper{
		Path:   "/rules",
		FS:     afero.NewMemMapFs(),
		logger: l,
	}

	t.Run("basic rulegroup", func(t *testing.T) {
		updated, files, err := m.MapRules(testUser, initialRuleSet)
		require.True(t, updated)
		require.Len(t, files, 1)
		require.Equal(t, fileOnePath, files[0])
		require.NoError(t, err)

		exists, err := afero.Exists(m.FS, fileOnePath)
		require.True(t, exists)
		require.NoError(t, err)
	})

	t.Run("identical rulegroup", func(t *testing.T) {
		updated, files, err := m.MapRules(testUser, initialRuleSet)
		require.False(t, updated)
		require.Len(t, files, 1)
		require.NoError(t, err)

		exists, err := afero.Exists(m.FS, fileOnePath)
		require.True(t, exists)
		require.NoError(t, err)
	})

	t.Run("out of order identical rulegroup", func(t *testing.T) {
		updated, files, err := m.MapRules(testUser, outOfOrderRuleSet)
		require.False(t, updated)
		require.Len(t, files, 1)
		require.NoError(t, err)

		exists, err := afero.Exists(m.FS, fileOnePath)
		require.True(t, exists)
		require.NoError(t, err)
	})

	t.Run("updated rulegroup", func(t *testing.T) {
		updated, files, err := m.MapRules(testUser, updatedRuleSet)
		require.True(t, updated)
		require.Len(t, files, 1)
		require.Equal(t, fileOnePath, files[0])
		require.NoError(t, err)

		exists, err := afero.Exists(m.FS, fileOnePath)
		require.True(t, exists)
		require.NoError(t, err)
	})
}

func Test_mapper_MapRulesMultipleFiles(t *testing.T) {
	l := log.NewLogfmtLogger(os.Stdout)
	l = level.NewFilter(l, level.AllowInfo())
	setupRuleSets()
	m := &mapper{
		Path:   "/rules",
		FS:     afero.NewMemMapFs(),
		logger: l,
	}

	t.Run("basic rulegroup", func(t *testing.T) {
		updated, files, err := m.MapRules(testUser, initialRuleSet)
		require.True(t, updated)
		require.Len(t, files, 1)
		require.Equal(t, fileOnePath, files[0])
		require.NoError(t, err)

		exists, err := afero.Exists(m.FS, fileOnePath)
		require.True(t, exists)
		require.NoError(t, err)
	})

	t.Run("add a file", func(t *testing.T) {
		updated, files, err := m.MapRules(testUser, twoFilesRuleSet)
		require.True(t, updated)
		require.Len(t, files, 2)
		require.True(t, sliceContains(t, fileOnePath, files))
		require.True(t, sliceContains(t, fileTwoPath, files))
		require.NoError(t, err)

		exists, err := afero.Exists(m.FS, fileOnePath)
		require.True(t, exists)
		require.NoError(t, err)
		exists, err = afero.Exists(m.FS, fileTwoPath)
		require.True(t, exists)
		require.NoError(t, err)
	})

	t.Run("update one file", func(t *testing.T) {
		updated, files, err := m.MapRules(testUser, twoFilesUpdatedRuleSet)
		require.True(t, updated)
		require.Len(t, files, 2)
		require.True(t, sliceContains(t, fileOnePath, files))
		require.True(t, sliceContains(t, fileTwoPath, files))
		require.NoError(t, err)

		exists, err := afero.Exists(m.FS, fileOnePath)
		require.True(t, exists)
		require.NoError(t, err)
		exists, err = afero.Exists(m.FS, fileTwoPath)
		require.True(t, exists)
		require.NoError(t, err)
	})

	t.Run("delete one file", func(t *testing.T) {
		updated, files, err := m.MapRules(testUser, twoFilesDeletedRuleSet)
		require.True(t, updated)
		require.Len(t, files, 1)
		require.Equal(t, fileOnePath, files[0])
		require.NoError(t, err)

		exists, err := afero.Exists(m.FS, fileOnePath)
		require.True(t, exists)
		require.NoError(t, err)
		exists, err = afero.Exists(m.FS, fileTwoPath)
		require.False(t, exists)
		require.NoError(t, err)
	})

}

func Test_mapper_MapRulesSpecialCharNamespace(t *testing.T) {
	l := log.NewLogfmtLogger(os.Stdout)
	l = level.NewFilter(l, level.AllowInfo())
	setupRuleSets()
	m := &mapper{
		Path:   "/rules",
		FS:     afero.NewMemMapFs(),
		logger: l,
	}

	t.Run("create special characters rulegroup", func(t *testing.T) {
		updated, files, err := m.MapRules(testUser, specialCharactersRuleSet)
		require.NoError(t, err)
		require.True(t, updated)
		require.Len(t, files, 1)
		require.Equal(t, specialCharFilePath, files[0])

		exists, err := afero.Exists(m.FS, specialCharFilePath)
		require.NoError(t, err)
		require.True(t, exists)
	})

	t.Run("delete special characters rulegroup", func(t *testing.T) {
		updated, files, err := m.MapRules(testUser, map[string][]rulefmt.RuleGroup{})
		require.NoError(t, err)
		require.True(t, updated)
		require.Len(t, files, 0)

		exists, err := afero.Exists(m.FS, specialCharFilePath)
		require.NoError(t, err)
		require.False(t, exists)
	})
}

func sliceContains(t *testing.T, find string, in []string) bool {
	t.Helper()

	for _, s := range in {
		if find == s {
			return true
		}
	}

	return false
}

func TestYamlFormatting(t *testing.T) {
	l := log.NewLogfmtLogger(os.Stdout)
	l = level.NewFilter(l, level.AllowInfo())
	setupRuleSets()

	m := &mapper{
		Path:   "/rules",
		FS:     afero.NewMemMapFs(),
		logger: l,
	}

	updated, files, err := m.MapRules(testUser, initialRuleSet)
	require.True(t, updated)
	require.Len(t, files, 1)
	require.Equal(t, fileOnePath, files[0])
	require.NoError(t, err)

	data, err := afero.ReadFile(m.FS, fileOnePath)
	require.NoError(t, err)

	expected := `groups:
    - name: rulegroup_two
      rules:
        - record: example_rule
          expr: example_expr
    - name: rulegroup_one
      rules:
        - record: example_rule
          expr: example_expr
`

	require.Equal(t, expected, string(data))
}
