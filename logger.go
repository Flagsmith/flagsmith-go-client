package flagsmith

import (
	"log"
	"os"
)

// Logger is the interface used for logging by flagsmith client. This interface defines the methods
// that a logger implementation must implement. It is used to abstract logging and
// enable clients to use any logger implementation they want.
type Logger interface {
	// Errorf logs an error message with the given format and arguments.
	Errorf(format string, v ...interface{})

	// Warnf logs a warning message with the given format and arguments.
	Warnf(format string, v ...interface{})

	// Debugf logs a debug message with the given format and arguments.
	Debugf(format string, v ...interface{})
}

func createLogger() *logger {
	l := &logger{l: log.New(os.Stderr, "", log.Ldate|log.Lmicroseconds)}
	return l
}

var _ Logger = (*logger)(nil)

type logger struct {
	l *log.Logger
}

func (l *logger) Errorf(format string, v ...interface{}) {
	l.output("ERROR FLAGSMITH: "+format, v...)
}

func (l *logger) Warnf(format string, v ...interface{}) {
	l.output("WARN FLAGSMITH: "+format, v...)
}

func (l *logger) Debugf(format string, v ...interface{}) {
	l.output("DEBUG FLAGSMITH: "+format, v...)
}

func (l *logger) output(format string, v ...interface{}) {
	if len(v) == 0 {
		l.l.Print(format)
		return
	}
	l.l.Printf(format, v...)
}
