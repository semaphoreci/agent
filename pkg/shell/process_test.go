package shell

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

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
	p := createProcess(b, fmt.Sprintf("for i in {0..500}; do echo '%s'; done", strings.Repeat("x", 1024)))
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
