// Portions of this file are derived from GoNavi's internal/app/app.go database
// cache at commit 60ea586c674259d4214ad7d7e75dddfa706e577a.
// Copyright 2026 Syngnat. Licensed under Apache-2.0.
// Modified for DB Access Gateway: resource-version/connection cache identity,
// generation-based flight invalidation, and a MySQL-only adapter factory.
package target

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/yogel/db-access-gateway/internal/control"
)

const dbCachePingInterval = 30 * time.Second

var (
	errConnectionReleased = errors.New("database connection request was released")
	errRegistryClosed     = errors.New("database registry is closed")
)

type cachedDatabase struct {
	inst       database
	lastPing   time.Time
	resourceID string
	version    uint64
}

type Registry struct {
	mu          sync.RWMutex
	cache       map[string]cachedDatabase
	generations map[string]uint64
	connects    singleflight.Group
	newDatabase func() database
	closed      bool
}

func NewRegistry() *Registry {
	return &Registry{
		cache:       make(map[string]cachedDatabase),
		generations: make(map[string]uint64),
		newDatabase: func() database { return &MySQLDB{} },
	}
}

func (r *Registry) Get(ctx context.Context, resource control.Resource) (*sql.DB, error) {
	config, err := connectionConfig(resource)
	if err != nil {
		return nil, err
	}
	instance, err := r.getDatabase(ctx, config)
	if err != nil {
		return nil, err
	}
	pool := instance.SQLDB()
	if pool == nil {
		return nil, errors.New("database adapter returned an empty SQL pool")
	}
	return pool, nil
}

func connectionConfig(resource control.Resource) (ConnectionConfig, error) {
	if strings.TrimSpace(resource.Username) == "" {
		return ConnectionConfig{}, errors.New("database username is not configured for resource")
	}
	password := resource.Password
	if password == "" {
		if strings.TrimSpace(resource.SecretRef) == "" {
			return ConnectionConfig{}, errors.New("database password is not configured for resource")
		}
		var err error
		password, err = resolveSecret(resource.SecretRef)
		if err != nil {
			return ConnectionConfig{}, err
		}
	}
	return ConnectionConfig{
		ResourceID: resource.ID, Version: resource.Version,
		Host: resource.Host, Port: resource.Port, Database: resource.DatabaseName,
		User: resource.Username, Password: password, TLSMode: resource.TLSMode,
		DialTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 15 * time.Second,
	}, nil
}

func getCacheKey(config ConnectionConfig) string {
	b, _ := json.Marshal(config)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func (r *Registry) getDatabase(ctx context.Context, config ConnectionConfig) (database, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	key := getCacheKey(config)

	r.mu.RLock()
	if r.closed {
		r.mu.RUnlock()
		return nil, errRegistryClosed
	}
	entry, exists := r.cache[key]
	generation := r.generations[config.ResourceID]
	r.mu.RUnlock()

	if exists && entry.inst != nil {
		if time.Since(entry.lastPing) < dbCachePingInterval {
			if err := r.databaseReturnError(key, entry.inst); err != nil {
				return nil, err
			}
			return entry.inst, nil
		}
		if err := entry.inst.Ping(ctx); err == nil {
			r.markHealthy(key, entry.inst, time.Now())
			if err := r.databaseReturnError(key, entry.inst); err != nil {
				return nil, err
			}
			return entry.inst, nil
		} else if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		r.removeIfCurrent(key, entry.inst)
		_ = entry.inst.Close()
	}

	groupKey := fmt.Sprintf("%s:%d", key, generation)
	resultCh := r.connects.DoChan(groupKey, func() (any, error) {
		return r.connectAndCache(context.WithoutCancel(ctx), config, key, generation)
	})
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case result := <-resultCh:
		if result.Err != nil {
			return nil, result.Err
		}
		instance, ok := result.Val.(database)
		if !ok || instance == nil {
			return nil, errors.New("database cache returned an invalid adapter")
		}
		if err := r.databaseReturnError(key, instance); err != nil {
			return nil, err
		}
		return instance, nil
	}
}

func (r *Registry) databaseReturnError(key string, instance database) error {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.closed {
		return errRegistryClosed
	}
	entry, exists := r.cache[key]
	if !exists || entry.inst == nil || entry.inst != instance {
		return errConnectionReleased
	}
	return nil
}

func (r *Registry) connectAndCache(ctx context.Context, config ConnectionConfig, key string, generation uint64) (database, error) {
	r.mu.RLock()
	if r.closed {
		r.mu.RUnlock()
		return nil, errRegistryClosed
	}
	if r.generations[config.ResourceID] != generation {
		r.mu.RUnlock()
		return nil, errConnectionReleased
	}
	if entry, ok := r.cache[key]; ok && entry.inst != nil {
		r.mu.RUnlock()
		return entry.inst, nil
	}
	r.mu.RUnlock()

	instance := r.newDatabase()
	if instance == nil {
		return nil, errors.New("database adapter factory returned nil")
	}
	connectCtx, cancel := context.WithTimeout(ctx, config.DialTimeout)
	defer cancel()
	if err := instance.Connect(connectCtx, config); err != nil {
		_ = instance.Close()
		return nil, err
	}

	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		_ = instance.Close()
		return nil, errRegistryClosed
	}
	if r.generations[config.ResourceID] != generation {
		r.mu.Unlock()
		_ = instance.Close()
		return nil, errConnectionReleased
	}
	if existing, ok := r.cache[key]; ok && existing.inst != nil {
		r.mu.Unlock()
		_ = instance.Close()
		return existing.inst, nil
	}
	r.cache[key] = cachedDatabase{inst: instance, lastPing: time.Now(), resourceID: config.ResourceID, version: config.Version}
	r.mu.Unlock()
	return instance, nil
}

func (r *Registry) markHealthy(key string, instance database, at time.Time) {
	r.mu.Lock()
	if entry, ok := r.cache[key]; ok && entry.inst == instance && at.After(entry.lastPing) {
		entry.lastPing = at
		r.cache[key] = entry
	}
	r.mu.Unlock()
}

func (r *Registry) removeIfCurrent(key string, instance database) {
	r.mu.Lock()
	if entry, ok := r.cache[key]; ok && entry.inst == instance {
		delete(r.cache, key)
	}
	r.mu.Unlock()
}

func (r *Registry) Invalidate(resourceID string) {
	var stale []database
	r.mu.Lock()
	r.generations[resourceID]++
	for key, entry := range r.cache {
		if entry.resourceID == resourceID {
			stale = append(stale, entry.inst)
			delete(r.cache, key)
		}
	}
	r.mu.Unlock()
	for _, instance := range stale {
		_ = instance.Close()
	}
}

func (r *Registry) Close() error {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return nil
	}
	r.closed = true
	instances := make([]database, 0, len(r.cache))
	for key, entry := range r.cache {
		instances = append(instances, entry.inst)
		delete(r.cache, key)
	}
	r.mu.Unlock()

	var joined error
	for _, instance := range instances {
		joined = errors.Join(joined, instance.Close())
	}
	return joined
}
