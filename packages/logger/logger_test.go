package logger

import (
	"bytes"
	"strings"
	"testing"
)

func TestLoggerLevelsAndOutput(t *testing.T) {
	var buf bytes.Buffer
	l := New(StdOptions{Out: &buf, Level: LevelDebug})
	l.Info("hello", "key", "val")
	l.Debug("debug", "k", "v")
	l.Warn("warn")
	l.Error("err")
	out := buf.String()
	for _, want := range []string{"hello", "debug", "warn", "err"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing level %q in output: %q", want, out)
		}
	}
	// Discard logger writes nothing.
	d := Discard()
	d.Info("x")
	if buf.Len() == 0 {
		t.Fatal("expected output from the non-discard logger")
	}
}
