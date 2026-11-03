package quickjswasm

import (
	"context"
	"testing"
	"time"
)

func BenchmarkOwnedRuntimeInitialization(b *testing.B) {
	e, err := New(context.Background(), Config{Timeout: 10 * time.Second})
	if err != nil {
		b.Fatal(err)
	}
	defer e.Close()
	b.ReportAllocs()
	for b.Loop() {
		m, err := e.instantiate(context.Background())
		if err != nil {
			b.Fatal(err)
		}
		m.Close(context.Background())
	}
}
func BenchmarkOwnedRuntimeClose(b *testing.B) {
	e, err := New(context.Background(), Config{Timeout: 10 * time.Second})
	if err != nil {
		b.Fatal(err)
	}
	defer e.Close()
	b.ReportAllocs()
	for b.Loop() {
		b.StopTimer()
		m, err := e.instantiate(context.Background())
		if err != nil {
			b.Fatal(err)
		}
		b.StartTimer()
		if err = m.Close(context.Background()); err != nil {
			b.Fatal(err)
		}
	}
}
