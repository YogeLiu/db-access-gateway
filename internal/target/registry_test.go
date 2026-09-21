// Tests adapted from GoNavi's internal/app/app_db_cache_concurrency_test.go.
// Copyright 2026 Syngnat. Licensed under Apache-2.0. Modified for this project.
package target

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yogel/db-access-gateway/internal/control"
)

type fakeDatabase struct {
	connect func(context.Context, ConnectionConfig) error
	closed  atomic.Int32
}

func (f *fakeDatabase) Connect(ctx context.Context, config ConnectionConfig) error {
	if f.connect != nil {
		return f.connect(ctx, config)
	}
	return nil
}
func (f *fakeDatabase) Close() error               { f.closed.Add(1); return nil }
func (f *fakeDatabase) Ping(context.Context) error { return nil }
func (f *fakeDatabase) SQLDB() *sql.DB             { return nil }

func testConfig() ConnectionConfig {
	return ConnectionConfig{
		ResourceID: "resource-a", Version: 1, Host: "mysql", Port: 3306,
		Database: "app", User: "reader", Password: "secret", TLSMode: "disabled", DialTimeout: time.Second,
	}
}

func TestRegistryCoalescesConcurrentColdConnects(t *testing.T) {
	const callers = 32
	r := NewRegistry()
	var factoryCalls atomic.Int32
	var connectCalls atomic.Int32
	started := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	r.newDatabase = func() database {
		factoryCalls.Add(1)
		return &fakeDatabase{connect: func(context.Context, ConnectionConfig) error {
			connectCalls.Add(1)
			once.Do(func() { close(started) })
			<-release
			return nil
		}}
	}

	start := make(chan struct{})
	results := make(chan database, callers)
	errs := make(chan error, callers)
	var workers sync.WaitGroup
	workers.Add(callers)
	for i := 0; i < callers; i++ {
		go func() {
			defer workers.Done()
			<-start
			instance, err := r.getDatabase(context.Background(), testConfig())
			results <- instance
			errs <- err
		}()
	}
	close(start)
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for physical connect")
	}
	time.Sleep(50 * time.Millisecond)
	close(release)
	workers.Wait()
	close(results)
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("getDatabase: %v", err)
		}
	}
	var shared database
	for instance := range results {
		if shared == nil {
			shared = instance
		} else if shared != instance {
			t.Fatal("callers did not receive one shared adapter")
		}
	}
	if factoryCalls.Load() != 1 || connectCalls.Load() != 1 {
		t.Fatalf("factory=%d connect=%d, want 1/1", factoryCalls.Load(), connectCalls.Load())
	}
}

func TestRegistryInvalidationRejectsInflightConnection(t *testing.T) {
	r := NewRegistry()
	started := make(chan struct{})
	release := make(chan struct{})
	first := &fakeDatabase{connect: func(context.Context, ConnectionConfig) error {
		close(started)
		<-release
		return nil
	}}
	second := &fakeDatabase{}
	var calls atomic.Int32
	r.newDatabase = func() database {
		if calls.Add(1) == 1 {
			return first
		}
		return second
	}

	errCh := make(chan error, 1)
	go func() {
		_, err := r.getDatabase(context.Background(), testConfig())
		errCh <- err
	}()
	<-started
	r.Invalidate("resource-a")
	close(release)
	if err := <-errCh; !errors.Is(err, errConnectionReleased) {
		t.Fatalf("old flight error=%v, want released", err)
	}
	if first.closed.Load() == 0 {
		t.Fatal("invalidated inflight adapter was not closed")
	}
	instance, err := r.getDatabase(context.Background(), testConfig())
	if err != nil {
		t.Fatal(err)
	}
	if instance != second {
		t.Fatal("new generation did not create a fresh adapter")
	}
}

func TestConnectionConfigUsesOneResourceCredential(t *testing.T) {
	t.Setenv("DB_SECRET_APP_PROD", "secret")
	config, err := connectionConfig(control.Resource{
		ID: "resource-a", Version: 1, Host: "mysql", Port: 3306, DatabaseName: "app",
		Username: "app_user", SecretRef: "app_prod", TLSMode: "disabled",
	})
	if err != nil {
		t.Fatal(err)
	}
	if config.User != "app_user" || config.Password != "secret" {
		t.Fatalf("unexpected credential: user=%q password=%q", config.User, config.Password)
	}
}
