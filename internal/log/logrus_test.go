package log

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anchore/go-logger"
	logrusadapter "github.com/anchore/go-logger/adapter/logrus"
)

func setLog(t *testing.T, l logger.Logger) {
	t.Helper()
	orig := Log
	Log = l
	t.Cleanup(func() { Log = orig })
}

func TestLogrusBridge_forwardsEntries(t *testing.T) {
	logFile := filepath.Join(t.TempDir(), "log.txt")
	l, err := logrusadapter.New(logrusadapter.Config{FileLocation: logFile, Level: logger.DebugLevel})
	require.NoError(t, err)
	setLog(t, l)

	lib := logrus.New()
	installLogrusBridge(lib, logger.DebugLevel)

	lib.WithField("errors", "xattr-err").Warn("from-library-warn")
	lib.Info("from-library-info")
	lib.WithField("from", "other").Error("from-library-error")

	contents, err := os.ReadFile(logFile)
	require.NoError(t, err)
	got := string(contents)
	assert.Contains(t, got, "from-library-warn")
	assert.Contains(t, got, "xattr-err")
	assert.Contains(t, got, "from-library-info")
	assert.Contains(t, got, "from-library-error")
	assert.Contains(t, got, "containers-storage")
	assert.NotContains(t, got, "other", "bridge tag should override an existing from field")

	assert.Equal(t, io.Discard, lib.Out)
}

func TestLogrusBridge_noRecursionWhenSharingLogger(t *testing.T) {
	// stereoscope's logger is backed by the same logrus logger being bridged
	lib := logrus.New()
	l, err := logrusadapter.Use(lib, logrusadapter.Config{EnableConsole: true, Level: logger.DebugLevel})
	require.NoError(t, err)
	setLog(t, l)

	installLogrusBridge(lib, logger.DebugLevel)

	// would overflow the stack without the recursion guard
	lib.Warn("does-not-recurse")
}

func TestLogrusLevel(t *testing.T) {
	tests := []struct {
		level logger.Level
		want  logrus.Level
	}{
		{logger.TraceLevel, logrus.DebugLevel},
		{logger.DebugLevel, logrus.DebugLevel},
		{logger.InfoLevel, logrus.WarnLevel},
		{logger.WarnLevel, logrus.WarnLevel},
		{logger.ErrorLevel, logrus.ErrorLevel},
		{logger.DisabledLevel, logrus.PanicLevel},
	}
	for _, tt := range tests {
		t.Run(string(tt.level), func(t *testing.T) {
			assert.Equal(t, tt.want, logrusLevel(tt.level))
		})
	}
}
