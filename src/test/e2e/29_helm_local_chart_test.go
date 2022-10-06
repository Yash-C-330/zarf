package test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLocalHelm(t *testing.T) {
	if !e2e.runClusterTests {
		t.Skip("")
	}
	t.Log("E2E: Local Helm chart")
	e2e.setup(t)
	defer e2e.teardown(t)

	path := fmt.Sprintf("build/zarf-package-test-helm-local-chart-%s.tar.zst", e2e.arch)

	// Deploy the charts
	stdOut, stdErr, err := e2e.execZarfCommand("package", "deploy", path, "--confirm")
	require.NoError(t, err, stdOut, stdErr)

	stdOut, stdErr, err = e2e.execZarfCommand("package", "remove", "test-helm-local-chart", "--confirm")
	require.NoError(t, err, stdOut, stdErr)
}
