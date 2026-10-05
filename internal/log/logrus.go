package log

import (
	"io"
	"sync"

	"github.com/sirupsen/logrus"

	"github.com/anchore/go-logger"
)

const (
	bridgeKey = "from"
	bridgeVal = "containers-storage"
)

var bridgeOnce sync.Once

// BridgeLogrus forwards entries from the global logrus logger into Log. Only the first call has any effect. The hook
// reads Log at fire time, so later changes to Log are picked up.
func BridgeLogrus(level logger.Level) {
	bridgeOnce.Do(func() {
		installLogrusBridge(logrus.StandardLogger(), level)
	})
}

// installLogrusBridge is split out so tests can target a logrus.New() instance instead of the global logger.
func installLogrusBridge(l *logrus.Logger, level logger.Level) {
	// add the hook before discarding output so no entries are lost in between
	l.AddHook(logrusHook{})
	// formatting entries that are written to io.Discard is wasted work
	l.SetFormatter(nopFormatter{})
	l.SetOutput(io.Discard)
	l.SetLevel(logrusLevel(level))
}

// logrusLevel picks the most verbose logrus level whose entries would still be visible at the given level, keeping in
// mind that the hook demotes library info entries to debug.
func logrusLevel(level logger.Level) logrus.Level {
	switch level {
	case logger.DebugLevel, logger.TraceLevel:
		return logrus.DebugLevel
	case logger.InfoLevel, logger.WarnLevel:
		return logrus.WarnLevel
	case logger.ErrorLevel:
		return logrus.ErrorLevel
	}
	return logrus.PanicLevel
}

type logrusHook struct{}

func (logrusHook) Levels() []logrus.Level {
	return logrus.AllLevels
}

func (logrusHook) Fire(entry *logrus.Entry) error {
	// guard against recursion when Log is itself backed by the same logrus logger: our own forwarded entry would come
	// back through this hook carrying the bridge field
	if entry.Data[bridgeKey] == bridgeVal {
		return nil
	}

	fields := make([]any, 0, 2*len(entry.Data)+2)
	for k, v := range entry.Data {
		fields = append(fields, k, v)
	}
	// set last so it wins over any existing "from" field
	fields = append(fields, bridgeKey, bridgeVal)

	l := Log.Nested(fields...)
	switch entry.Level {
	case logrus.PanicLevel, logrus.FatalLevel, logrus.ErrorLevel:
		l.Error(entry.Message)
	case logrus.WarnLevel:
		l.Warn(entry.Message)
	default:
		// containers libraries are chatty at info, which is debug-level detail from stereoscope's point of view (trace
		// entries never reach here since the logger level is at most debug)
		l.Debug(entry.Message)
	}
	return nil
}

type nopFormatter struct{}

func (nopFormatter) Format(*logrus.Entry) ([]byte, error) {
	return nil, nil
}
