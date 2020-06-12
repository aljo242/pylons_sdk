package evtesting

import (
	"fmt"
	"runtime"
	"testing"

	log "github.com/sirupsen/logrus"

	"github.com/stretchr/testify/require"
)

// T is a modified testing.T
type T struct {
	origin    *testing.T
	useLogPkg bool
	fields    log.Fields
	logLevel  log.Level
}

// Fields is a type to manage json based output
type Fields log.Fields

var listeners = make(map[string]func())

// NewT is function returns modified T from original testing.T
func NewT(origin *testing.T) T {
	newT := T{
		origin:    origin,
		useLogPkg: false,
		fields:    log.Fields{},
		logLevel:  log.DebugLevel,
	}
	if origin == nil {
		orgT := testing.T{}
		newT.origin = &orgT
		newT.useLogPkg = true
		newT.logLevel = log.TraceLevel
	}
	return newT
}

// WithFields is to manage data in json format
func (t *T) WithFields(fields Fields) *T {
	return &T{
		fields:    log.Fields(fields),
		origin:    t.origin,
		useLogPkg: t.useLogPkg,
		logLevel:  t.logLevel,
	}
}

// Run is modified Run
func (t *T) Run(name string, f func(t *T)) bool {
	return t.origin.Run(name, func(subt *testing.T) {
		newT := T{
			origin:    subt,
			fields:    t.fields,
			useLogPkg: t.useLogPkg,
			logLevel:  t.logLevel,
		}
		f(&newT)
	})
}

// DispatchEvent process events that are related to the event e.g. failure in one test case make others to fail without continuing
func (t *T) DispatchEvent(event string) {
	if listener, ok := listeners[event]; ok {
		listener()
	}
}

func getFrame(skipFrames int) runtime.Frame {
	// We need the frame at index skipFrames+2, since we never want runtime.Callers and getFrame
	targetFrameIndex := skipFrames + 2

	// Set size to targetFrameIndex+2 to ensure we have room for one more caller than we need
	programCounters := make([]uintptr, targetFrameIndex+2)
	n := runtime.Callers(0, programCounters)

	frame := runtime.Frame{Function: "unknown"}
	if n > 0 {
		frames := runtime.CallersFrames(programCounters[:n])
		for more, frameIndex := true, 0; more && frameIndex <= targetFrameIndex; frameIndex++ {
			var frameCandidate runtime.Frame
			frameCandidate, more = frames.Next()
			if frameIndex == targetFrameIndex {
				frame = frameCandidate
			}
		}
	}

	return frame
}

func (t *T) printCallerLine() {
	frame := getFrame(2)
	if t.useLogPkg {
		text := fmt.Sprintf("%s:%d %s", frame.File, frame.Line, frame.Function)
		log.WithFields(log.Fields{
			"file_line": fmt.Sprintf("%s:%d", frame.File, frame.Line),
			"func":      frame.Function,
		}).Trace(text)
	} else {
		nT := t.WithFields(Fields{
			"file_line": fmt.Sprintf("%s:%d", frame.File, frame.Line),
			"func":      frame.Function,
		})
		t.origin.Log(nT.FormatFields())
	}
}

// FormatFields renders a single log entry
func (t *T) FormatFields() string {
	var formated string
	data := make(Fields)
	for k, v := range t.fields {
		data[k] = v
	}

	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}

	fixedKeys := []string{}
	fixedKeys = append(fixedKeys, keys...)

	for _, key := range fixedKeys {
		formated += fmt.Sprintf(" %s=%+v", key, data[key])
	}

	return formated
}

// Fatal is a modified Fatal
func (t *T) Fatal(args ...interface{}) {
	t.DispatchEvent("FAIL")
	t.printCallerLine()
	if t.useLogPkg {
		log.WithFields(t.fields).Fatal(args...)
	} else {
		t.origin.Log(t.FormatFields())
		t.origin.Fatal(args...)
	}
}

// Fatalf is a modified Fatalf
func (t *T) Fatalf(format string, args ...interface{}) {
	t.DispatchEvent("FAIL")
	t.printCallerLine()
	if t.useLogPkg {
		log.WithFields(t.fields).Fatalf(format, args...)
	} else {
		t.origin.Log(t.FormatFields())
		t.origin.Fatalf(format, args...)
	}
}

// MustTrue validate if value is true
func (t *T) MustTrue(value bool) {
	if !value {
		t.DispatchEvent("FAIL")
	}
	if t.useLogPkg {
		if !value {
			t.printCallerLine()
			log.Fatal("MustTrue validation failure")
		}
	} else {
		require.True(t.origin, value)
	}
}

// MustNil validate if value is nil
func (t *T) MustNil(err error) {
	if err != nil {
		t.DispatchEvent("FAIL")
		if t.useLogPkg {
			t.printCallerLine()
			t.WithFields(Fields{
				"error": err,
			}).Fatal("MustNil validation failure")
		} else {
			require.True(t.origin, err == nil)
		}
	}
}

// Parallel is modified Parallel
func (t *T) Parallel() {
	t.origin.Parallel()
}

// Log is modified Log
func (t *T) Log(args ...interface{}) {
	if t.useLogPkg {
		log.WithFields(t.fields).Infoln(args...)
	} else {
		t.origin.Log(t.FormatFields())
		t.origin.Log(args...)
	}
}

// Info is modified Info
func (t *T) Info(args ...interface{}) {
	if t.logLevel < log.InfoLevel {
		return
	}
	if t.useLogPkg {
		log.WithFields(t.fields).Infoln(args...)
	} else {
		t.origin.Log(t.FormatFields())
		t.origin.Log(args...)
	}
}

// Warn is modified Info
func (t *T) Warn(args ...interface{}) {
	if t.logLevel < log.WarnLevel {
		return
	}
	t.printCallerLine()
	if t.useLogPkg {
		log.WithFields(t.fields).Warnln(args...)
	} else {
		t.origin.Log(t.FormatFields())
		t.origin.Log(args...)
	}
}

// Trace is modified Trace
func (t *T) Trace(args ...interface{}) {
	if t.logLevel < log.TraceLevel {
		return
	}
	t.printCallerLine()
	if t.useLogPkg {
		log.WithFields(t.fields).Traceln(args...)
	} else {
		t.origin.Log(t.FormatFields())
		t.origin.Log(args...)
	}
}

// Debug is modified Debug
func (t *T) Debug(args ...interface{}) {
	if t.logLevel < log.DebugLevel {
		return
	}
	t.printCallerLine()
	if t.useLogPkg {
		log.WithFields(t.fields).Debugln(args...)
	} else {
		t.origin.Log(t.FormatFields())
		t.origin.Log(args...)
	}
}
