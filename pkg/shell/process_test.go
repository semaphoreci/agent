package shell

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// scanHarness drives Process.scan() without a real bash/PTY by replacing the
// process's read source with a channel. Each chunk the test writes is returned
// as exactly one read() call, so read boundaries are deterministic - no sleeps,
// no reliance on pipe scheduling. scan runs in a goroutine so a lost end marker
// shows up as a timeout instead of hanging the test process.
//
// Handshake: before every read, scan signals readReady, then blocks receiving
// the next chunk. write() waits for that signal before sending, guaranteeing
// the chunk lands in the read scan is about to perform. While scan is parked
// waiting for a chunk it is not touching inputBuffer, so a test may inspect
// inputBuffer between readReady and the next write without racing the scanner.
type scanHarness struct {
	p         *Process
	scanErr   chan error
	chunks    chan []byte
	readReady chan struct{}
	closeOnce sync.Once

	outputMu sync.Mutex
	output   strings.Builder
}

func newScanHarness(t *testing.T) *scanHarness {
	t.Helper()

	h := &scanHarness{
		scanErr:   make(chan error, 1),
		chunks:    make(chan []byte),
		readReady: make(chan struct{}, 1),
	}

	shell := &Shell{ExitSignal: make(chan string, 1)}
	h.p = NewProcess(Config{
		Shell:       shell,
		StoragePath: os.TempDir(),
		OnOutput: func(s string) {
			h.outputMu.Lock()
			h.output.WriteString(s)
			h.outputMu.Unlock()
		},
	})

	h.p.readSource = func() ([]byte, error) {
		h.readReady <- struct{}{}
		chunk, ok := <-h.chunks
		if !ok {
			return nil, io.EOF
		}
		return chunk, nil
	}

	// If a test bails out before feeding a completing marker, unblock the
	// parked scanner so its goroutine exits instead of leaking.
	t.Cleanup(func() {
		h.closeOnce.Do(func() { close(h.chunks) })
	})

	return h
}

func (h *scanHarness) run() {
	go func() { h.scanErr <- h.p.scan() }()
}

// awaitRead blocks until scan is parked waiting for its next read().
func (h *scanHarness) awaitRead() {
	<-h.readReady
}

// feed hands scan the bytes for the read it is currently waiting on. It must be
// called only after awaitRead (write() does both).
func (h *scanHarness) feed(chunk string) {
	h.chunks <- []byte(chunk)
}

func (h *scanHarness) write(t *testing.T, chunk string) {
	t.Helper()
	h.awaitRead()
	h.feed(chunk)
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

// When a stray \001, ordinary command output, and the complete end marker all
// arrive in a single read, scan must locate the *whole* marker (not the first
// \001) so the output before the marker is published rather than discarded.
// Only the marker itself is stripped.
func Test__Scan_PreservesOutputAroundStrayControlByteInSameRead(t *testing.T) {
	h := newScanHarness(t)
	p := h.p
	h.run()

	h.write(t, "\001 "+p.startMark+"\r\n")

	// One read: a stray \001 inside ordinary output, followed by the complete
	// real end marker. "hello\001world " is genuine output; the marker begins
	// at the second \001 (the only \001 followed by " <endMark>").
	h.write(t, "hello\001world \001 "+p.endMark+" 0\r\n")

	select {
	case err := <-h.scanErr:
		if err != nil {
			t.Fatalf("scan returned error: %v", err)
		}
		if p.ExitCode != 0 {
			t.Fatalf("expected exit code 0, got %d", p.ExitCode)
		}
		out := h.collectedOutput()
		if !strings.Contains(out, "hello\001world ") {
			t.Fatalf("ordinary output around the stray \\001 was lost: %q", out)
		}
		if strings.Contains(out, p.endMark) {
			t.Fatalf("end marker leaked into job output: %q", out)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("scan did not complete when a stray \\001 preceded the end marker in one read")
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

	// Wait until scan has consumed all of the above and parked on the next
	// read. It is not touching inputBuffer now, so this read is race-free.
	h.awaitRead()
	if got := len(p.inputBuffer); got > p.maxEndMarkerLen() {
		t.Fatalf("input buffer not bounded: %d bytes retained (max %d)", got, p.maxEndMarkerLen())
	}

	// A real marker still completes afterwards.
	h.feed("\001 " + p.endMark + " 0\r\n")

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
