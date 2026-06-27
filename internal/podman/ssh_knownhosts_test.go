package podman

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/anchore/go-logger"
	"github.com/anchore/go-logger/adapter/discard"
	"github.com/anchore/stereoscope/internal/log"
	"github.com/anchore/stereoscope/internal/testutil"
)

// capturingLogger records every Errorf call so a test can assert that
// hostKey did not log anything while parsing a known_hosts file. The
// embedded discard logger satisfies the rest of the logger.Logger
// interface so only Error/Errorf need overriding.
type capturingLogger struct {
	logger.Logger
	errors []string
}

func newCapturingLogger() *capturingLogger {
	return &capturingLogger{Logger: discard.New()}
}

func (l *capturingLogger) Errorf(format string, _ ...any) {
	l.errors = append(l.errors, format)
}

func (l *capturingLogger) Error(args ...any) {
	l.errors = append(l.errors, "error")
	_ = args
}

func TestHostKey_commentsDoNotLogErrors(t *testing.T) {
	prev := log.Log
	cl := newCapturingLogger()
	log.Log = cl
	t.Cleanup(func() { log.Log = prev })

	knownHostsPath := testutil.GetFixturePath(t, "known_hosts_comments")

	pk := hostKey("github.com", knownHostsPath)

	assert.NotNil(t, pk, "expected the real host entry to be parsed past the comment and blank line")
	assert.Equal(t, "ssh-rsa", pk.Type())
	assert.Empty(t, cl.errors, "comment and blank lines must not produce parse-error logs")
}
