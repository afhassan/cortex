//go:build requires_docker
// +build requires_docker

package integration

import (
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thanos-io/thanos/pkg/runutil"

	"github.com/cortexproject/cortex/integration/e2e"
	"github.com/cortexproject/cortex/integration/e2ecortex"
)

func TestIndexAPIEndpoint(t *testing.T) {
	s, err := e2e.NewScenario(networkName)
	require.NoError(t, err)
	defer s.Close()

	configOverrides := map[string]string{
		// alert manager
		"-alertmanager.web.external-url":   "http://localhost/alertmanager",
		"-alertmanager-storage.backend":    "local",
		"-alertmanager-storage.local.path": filepath.Join(e2e.ContainerSharedDir, "alertmanager_configs"),
	}
	// make alert manager config dir
	require.NoError(t, writeFileToSharedDir(s, "alertmanager_configs", []byte{}))

	// Start Cortex in single binary mode, reading the config from file.
	require.NoError(t, copyFileToSharedDir(s, "docs/configuration/single-process-config-blocks-local.yaml", cortexConfigFile))

	cortex1 := e2ecortex.NewSingleBinaryWithConfigFile("cortex-1", cortexConfigFile, configOverrides, "", 9009, 9095)
	require.NoError(t, s.StartAndWaitReady(cortex1))

	// GET / should succeed
	res, err := e2e.GetRequest(fmt.Sprintf("http://%s", cortex1.Endpoint(9009)))
	require.NoError(t, err)
	assert.Equal(t, 200, res.StatusCode)

	// POST / should fail
	res, err = e2e.PostRequest(fmt.Sprintf("http://%s", cortex1.Endpoint(9009)))
	require.NoError(t, err)
	assert.Equal(t, 405, res.StatusCode)
}

func TestConfigAPIEndpoint(t *testing.T) {
	s, err := e2e.NewScenario(networkName)
	require.NoError(t, err)
	defer s.Close()

	configOverrides := map[string]string{
		// alert manager
		"-alertmanager.web.external-url":   "http://localhost/alertmanager",
		"-alertmanager-storage.backend":    "local",
		"-alertmanager-storage.local.path": filepath.Join(e2e.ContainerSharedDir, "alertmanager_configs"),
	}
	// make alert manager config dir
	require.NoError(t, writeFileToSharedDir(s, "alertmanager_configs", []byte{}))

	// Start Cortex in single binary mode, reading the config from file.
	require.NoError(t, copyFileToSharedDir(s, "docs/configuration/single-process-config-blocks-local.yaml", cortexConfigFile))

	cortex1 := e2ecortex.NewSingleBinaryWithConfigFile("cortex-1", cortexConfigFile, configOverrides, "", 9009, 9095)
	require.NoError(t, s.StartAndWaitReady(cortex1))

	// Get config from /config API endpoint.
	res, err := e2e.GetRequest(fmt.Sprintf("http://%s/config", cortex1.Endpoint(9009)))
	require.NoError(t, err)

	defer runutil.ExhaustCloseWithErrCapture(&err, res.Body, "config API response")
	body, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, res.StatusCode)

	// Start again Cortex in single binary with the exported config
	// and ensure it starts (pass the readiness probe).
	require.NoError(t, writeFileToSharedDir(s, cortexConfigFile, body))
	configOverrides["-alertmanager.cluster.peers"] = cortex1.HTTPEndpoint()
	cortex2 := e2ecortex.NewSingleBinaryWithConfigFile("cortex-2", cortexConfigFile, configOverrides, "", 9009, 9095)
	require.NoError(t, s.StartAndWaitReady(cortex2))
}
