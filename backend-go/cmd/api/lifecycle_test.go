package main

import (
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

func TestSIGTERMTriggersShutdownHookAndStopsNewRequests(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var requests atomic.Int32
	server := &http.Server{Handler: http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests.Add(1) })}
	hookCalled := make(chan struct{})
	server.RegisterOnShutdown(func() { close(hookCalled) })
	signals := make(chan os.Signal, 1)
	result := make(chan error, 1)
	go func() {
		result <- serveAPIOnListenerUntilShutdown(server, listener, signals, time.Second, nil, log.New(io.Discard, "", 0))
	}()
	signals <- syscall.SIGTERM
	if err := <-result; err != nil {
		t.Fatal(err)
	}
	select {
	case <-hookCalled:
	case <-time.After(time.Second):
		t.Fatal("RegisterOnShutdown hook was not called")
	}
	before := requests.Load()
	client := &http.Client{Timeout: 100 * time.Millisecond}
	_, _ = client.Get("http://" + listener.Addr().String())
	if requests.Load() != before {
		t.Fatal("server accepted a new request after shutdown")
	}
}

func TestShutdownCoordinatorTimeoutAndIdempotency(t *testing.T) {
	requestStarted := make(chan struct{})
	releaseRequest := make(chan struct{})
	server := &http.Server{Handler: http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		close(requestStarted)
		<-releaseRequest
	})}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = server.Serve(listener) }()
	go func() { _, _ = http.Get("http://" + listener.Addr().String()) }()
	select {
	case <-requestStarted:
	case <-time.After(time.Second):
		t.Fatal("request did not start")
	}
	var stopped atomic.Int32
	coordinator := newAPIShutdownCoordinator(server, 10*time.Millisecond, func() { stopped.Add(1) }, log.New(io.Discard, "", 0))
	first := coordinator.Shutdown("timeout")
	second := coordinator.Shutdown("duplicate")
	close(releaseRequest)
	if first == nil || second == nil {
		t.Fatal("timeout must be returned")
	}
	if stopped.Load() != 1 {
		t.Fatalf("shutdown was not idempotent: stop calls=%d", stopped.Load())
	}
}

func TestShutdownStepRecoversPanic(t *testing.T) {
	err := runShutdownStep("panic cleanup", func() error { panic("boom") })
	if err == nil {
		t.Fatal("cleanup panic must be converted to an error")
	}
}

func serveAPIOnListenerUntilShutdown(server *http.Server, listener net.Listener, signals <-chan os.Signal, timeout time.Duration, stopWorkers func(), logger *log.Logger) error {
	coordinator := newAPIShutdownCoordinator(server, timeout, stopWorkers, logger)
	result := make(chan error, 1)
	go func() { result <- server.Serve(listener) }()
	select {
	case signalValue := <-signals:
		if err := coordinator.Shutdown(signalValue.String()); err != nil {
			return err
		}
		err := <-result
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	case err := <-result:
		return err
	}
}
