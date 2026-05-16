package server

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"sync/atomic"
	"testing"
	"time"
)

// runWithShutdown must let an inflight handler finish before returning,
// even after ctx cancellation. Regression guard for Server.Start's drain path.
func TestRunWithShutdown_DrainsInflightRequest(t *testing.T) {
	t.Parallel()

	started := make(chan struct{})
	release := make(chan struct{})
	var finished int32

	mux := http.NewServeMux()
	mux.HandleFunc("/slow", func(w http.ResponseWriter, r *http.Request) {
		close(started)
		select {
		case <-release:
			atomic.AddInt32(&finished, 1)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("done"))
		case <-time.After(5 * time.Second):
			t.Errorf("handler timed out — never released")
		}
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()

	srv := &http.Server{Addr: addr, Handler: mux}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runReturned := make(chan error, 1)
	go func() {
		runReturned <- runWithShutdown(ctx, srv, logger, 3*time.Second)
	}()

	// Wait for the listener to accept connections.
	if err := waitForListen(addr, 1*time.Second); err != nil {
		t.Fatalf("server never opened the port: %v", err)
	}

	// Fire a slow request — blocks in the handler until release is closed.
	reqDone := make(chan error, 1)
	var body []byte
	go func() {
		resp, err := http.Get("http://" + addr + "/slow")
		if err != nil {
			reqDone <- err
			return
		}
		defer func() { _ = resp.Body.Close() }()
		body, _ = io.ReadAll(resp.Body)
		reqDone <- nil
	}()

	// Wait for handler entry — eliminates the sleep-based flake window.
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("handler never started")
	}

	cancel()

	// Inflight request must NOT return yet — drain is in progress.
	select {
	case err := <-reqDone:
		t.Fatalf("inflight request returned during drain: err=%v body=%q", err, body)
	case <-time.After(100 * time.Millisecond):
	}

	close(release)

	select {
	case err := <-reqDone:
		if err != nil {
			t.Fatalf("inflight request error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("inflight request never completed after release")
	}

	if string(body) != "done" {
		t.Fatalf("body: got %q want %q", string(body), "done")
	}
	if atomic.LoadInt32(&finished) != 1 {
		t.Fatalf("handler finished: got %d want 1", atomic.LoadInt32(&finished))
	}

	select {
	case err := <-runReturned:
		if err != nil {
			t.Fatalf("runWithShutdown returned: %v", err)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("runWithShutdown blocked beyond handler completion")
	}
}

func waitForListen(addr string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		c, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
		if err == nil {
			_ = c.Close()
			return nil
		}
		time.Sleep(10 * time.Millisecond)
	}
	return context.DeadlineExceeded
}
