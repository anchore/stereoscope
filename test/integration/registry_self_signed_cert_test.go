package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRegistrySelfSignedCert(t *testing.T) {
	cwd, err := os.Getwd()
	require.NoErrorf(t, err, "unable to get cwd: %+v", err)
	fixturesPath := filepath.Join(cwd, "test-fixtures", "registry")

	runMakeTarget := func(targets ...string) func(*testing.T) {
		return func(t *testing.T) {
			t.Logf("Running make targets %s", targets)

			cmd := exec.Command("make", targets...)
			cmd.Dir = fixturesPath
			runAndShow(t, cmd)
		}
	}

	tests := []struct {
		name    string
		setup   func(*testing.T)
		run     func(*testing.T)
		cleanup func(*testing.T)
	}{
		{
			name:    "go case",
			setup:   runMakeTarget(),
			run:     runMakeTarget("run"),
			cleanup: runMakeTarget("stop"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Cleanup(func() {
				tt.cleanup(t)
			})
			tt.setup(t)
			tt.run(t)
		})
	}
}
