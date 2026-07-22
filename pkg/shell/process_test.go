package shell

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// scanHarness drives Process.scan() without a real bash/PTY. The shell's TTY
// is an os.Pipe, so a test can script exactly which bytes arrive in each read
// (by writing a chunk, then pausing long enough for scan to consume it and
// block on the next read). scan runs in a goroutine so a lost end marker shows
// up as a timeout instead of hanging the test process.
type scanHarness struct {
	p        *Process
	reader   *os.File
	writer   *os.File
	scanErr  chan error
	outputMu sync.Mutex
	output   strings.Builder
}

func newScanHarness(t *testing.T) *scanHarness {
	t.Helper()

	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}

	h := &scanHarness{
		reader:  reader,
		writer:  writer,
		scanErr: make(chan error, 1),
	}

	shell := &Shell{TTY: reader, ExitSignal: make(chan string, 1)}
	h.p = NewProcess(Config{
		Shell:       shell,
		StoragePath: os.TempDir(),
		OnOutput: func(s string) {
			h.outputMu.Lock()
			h.output.WriteString(s)
			h.outputMu.Unlock()
		},
	})

	t.Cleanup(func() {
		_ = h.writer.Close()
		_ = h.reader.Close()
	})

	return h
}

func (h *scanHarness) run() {
	go func() { h.scanErr <- h.p.scan() }()
}

func (h *scanHarness) write(t *testing.T, chunk string) {
	t.Helper()
	if _, err := h.writer.WriteString(chunk); err != nil {
		t.Fatalf("writing chunk to pty pipe: %v", err)
	}
	// Give scan time to consume this chunk and block on the next read, so the
	// following chunk lands in a separate read() call.
	time.Sleep(60 * time.Millisecond)
}

func (h *scanHarness) collectedOutput() string {
	h.outputMu.Lock()
	defer h.outputMu.Unlock()
	return h.output.String()
}

// A stray 0x01 byte in the command's own output (e.g. binary data printed to
// the log) must not cause the real end marker to be flushed to the job output
// and lost. Before the fix, the buffering-mode flush dumped the whole buffer -
// including a partially-received real marker sitting after the stray byte - so
// the marker never appeared whole and scan() looped forever.
func Test__Scan_CompletesWhenStrayControlBytePrecedesEndMarker(t *testing.T) {
	h := newScanHarness(t)
	p := h.p
	h.run()

	// Start marker so scan enters the end-marker loop.
	h.write(t, "\001 "+p.startMark+"\r\n")

	// Command output: a stray \001, padding that pushes the buffer past the
	// flush threshold, then the *partial* real end marker (no exit code yet).
	h.write(t, "\001"+strings.Repeat("X", 20)+"\001 "+p.endMark)

	// Remainder of the real end marker arrives in the next read.
	h.write(t, " 0\r\n")

	select {
	case err := <-h.scanErr:
		if err != nil {
			t.Fatalf("scan returned error: %v", err)
		}
		if p.ExitCode != 0 {
			t.Fatalf("expected exit code 0, got %d", p.ExitCode)
		}
		if strings.Contains(h.collectedOutput(), p.endMark) {
			t.Fatalf("end marker leaked into job output: %q", h.collectedOutput())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("scan did not complete: stray \\001 caused the end marker to be lost and the command hung")
	}
}

// A pathological stream of \001 bytes with no completing marker must not grow
// the input buffer without bound: the retained partial-marker tail is capped.
func Test__Scan_BoundsBufferOnRepeatedControlBytes(t *testing.T) {
	h := newScanHarness(t)
	p := h.p
	h.run()

	h.write(t, "\001 "+p.startMark+"\r\n")

	// Many \001 bytes separated by junk longer than any real marker, none of
	// which forms a complete end marker.
	for i := 0; i < 5; i++ {
		h.write(t, "\001"+strings.Repeat("Y", len(p.endMark)+20))
	}

	if got := len(p.inputBuffer); got > len(p.endMark)+10 {
		t.Fatalf("input buffer not bounded: %d bytes retained (max %d)", got, len(p.endMark)+10)
	}

	// A real marker still completes afterwards.
	h.write(t, "\001 "+p.endMark+" 0\r\n")

	select {
	case err := <-h.scanErr:
		if err != nil {
			t.Fatalf("scan returned error: %v", err)
		}
		if p.ExitCode != 0 {
			t.Fatalf("expected exit code 0, got %d", p.ExitCode)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("scan did not complete after repeated control bytes")
	}
}

// The start marker may arrive terminated by a bare \n (not \r\n) when a
// previous command left the PTY in -onlcr mode. It must still be recognized.
func Test__Scan_CompletesWhenStartMarkerHasBareNewline(t *testing.T) {
	h := newScanHarness(t)
	p := h.p
	h.run()

	// Start marker terminated by a bare \n rather than \r\n.
	h.write(t, "\001 "+p.startMark+"\n")

	h.write(t, "hello\r\n\001 "+p.endMark+" 0\r\n")

	select {
	case err := <-h.scanErr:
		if err != nil {
			t.Fatalf("scan returned error: %v", err)
		}
		if p.ExitCode != 0 {
			t.Fatalf("expected exit code 0, got %d", p.ExitCode)
		}
		if !strings.Contains(h.collectedOutput(), "hello") {
			t.Fatalf("expected command output to contain 'hello', got %q", h.collectedOutput())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("scan did not complete: start marker with a bare newline was not recognized")
	}
}

func Benchmark__CommandOutput_128Bytes(b *testing.B) {
	p := createProcess(b, fmt.Sprintf("echo '%s'", strings.Repeat("x", 128)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		p.Run()
	}
}

func Benchmark__CommandOutput_1K(b *testing.B) {
	p := createProcess(b, fmt.Sprintf("echo '%s'", strings.Repeat("x", 1024)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		p.Run()
	}
}

func Benchmark__CommandOutput_10K(b *testing.B) {
	p := createProcess(b, fmt.Sprintf("for i in {0..10}; do echo '%s'; done", strings.Repeat("x", 1024)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		p.Run()
	}
}

func Benchmark__CommandOutput_100K(b *testing.B) {
	p := createProcess(b, fmt.Sprintf("for i in {0..100}; do echo '%s'; done", strings.Repeat("x", 1024)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		p.Run()
	}
}

func Benchmark__CommandOutput_250K(b *testing.B) {
	p := createProcess(b, fmt.Sprintf("for i in {0..250}; do echo '%s'; done", strings.Repeat("x", 1024)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		p.Run()
	}
}

func Benchmark__CommandOutput_500K(b *testing.B) {
	p := createProcess(b, fmt.Sprintf("for i in {0..500}; do echo '%s'; done", strings.Repeat("x", 1024)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		p.Run()
	}
}

func Benchmark__CommandOutput_1M(b *testing.B) {
	p := createProcess(b, fmt.Sprintf("for i in {0..1000}; do echo '%s'; done", strings.Repeat("x", 1024)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		p.Run()
	}
}

func createProcess(b *testing.B, command string) *Process {
	s, err := NewShell(os.TempDir())
	if err != nil {
		b.Fatalf("error creating shell: %v", err)
	}

	err = s.Start()
	if err != nil {
		b.Fatalf("error creating shell: %v", err)
	}

	return NewProcess(Config{
		Shell:       s,
		StoragePath: os.TempDir(),
		Command:     command,
		OnOutput:    func(string) { /* discard output */ },
	})
}
