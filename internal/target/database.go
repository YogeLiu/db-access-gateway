// Portions of this file are derived from GoNavi's internal/db/mysql_impl.go
// and internal/db/sql_pool.go at commit 60ea586c674259d4214ad7d7e75dddfa706e577a.
// Copyright 2026 Syngnat. Licensed under Apache-2.0.
// Modified for DB Access Gateway: MySQL-only configuration, context-aware
// methods, mandatory single statements, and no SSH/desktop dependencies.
package target

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/go-sql-driver/mysql"
)

const (
	defaultSQLMaxOpenConns    = 4
	defaultSQLMaxIdleConns    = 0
	defaultSQLConnMaxLifetime = 30 * time.Minute
	defaultSQLConnMaxIdleTime = 30 * time.Second
)

type ConnectionConfig struct {
	ResourceID   string
	Version      uint64
	Mode         string
	Host         string
	Port         uint16
	Database     string
	User         string
	Password     string
	TLSMode      string
	DialTimeout  time.Duration
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
}

type database interface {
	Connect(context.Context, ConnectionConfig) error
	Close() error
	Ping(context.Context) error
	SQLDB() *sql.DB
}

type MySQLDB struct {
	conn        *sql.DB
	pingTimeout time.Duration
}

func configureSQLConnectionPool(db *sql.DB) {
	if db == nil {
		return
	}
	db.SetMaxOpenConns(defaultSQLMaxOpenConns)
	db.SetMaxIdleConns(defaultSQLMaxIdleConns)
	db.SetConnMaxIdleTime(defaultSQLConnMaxIdleTime)
	db.SetConnMaxLifetime(defaultSQLConnMaxLifetime)
}

func (m *MySQLDB) Connect(ctx context.Context, config ConnectionConfig) error {
	if m == nil {
		return errors.New("mysql adapter is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	dialTimeout := config.DialTimeout
	if dialTimeout <= 0 {
		dialTimeout = 5 * time.Second
	}
	readTimeout := config.ReadTimeout
	if readTimeout <= 0 {
		readTimeout = 15 * time.Second
	}
	writeTimeout := config.WriteTimeout
	if writeTimeout <= 0 {
		writeTimeout = 15 * time.Second
	}

	driverConfig := mysql.NewConfig()
	driverConfig.Net = "tcp"
	driverConfig.Addr = net.JoinHostPort(config.Host, fmt.Sprintf("%d", config.Port))
	driverConfig.User = config.User
	driverConfig.Passwd = config.Password
	driverConfig.DBName = config.Database
	driverConfig.ParseTime = true
	driverConfig.Loc = time.Local
	driverConfig.MultiStatements = false
	driverConfig.InterpolateParams = false
	driverConfig.AllowAllFiles = false
	driverConfig.Timeout = dialTimeout
	driverConfig.ReadTimeout = readTimeout
	driverConfig.WriteTimeout = writeTimeout
	driverConfig.Collation = "utf8mb4_unicode_ci"
	driverConfig.Params = map[string]string{"charset": "utf8mb4"}

	switch config.TLSMode {
	case "required":
		driverConfig.TLSConfig = "true"
	case "skip_verify":
		driverConfig.TLSConfig = "skip-verify"
	case "preferred":
		driverConfig.TLSConfig = "preferred"
	case "disabled":
		driverConfig.TLSConfig = "false"
	default:
		return errors.New("unsupported TLS mode")
	}

	db, err := sql.Open("mysql", driverConfig.FormatDSN())
	if err != nil {
		return errors.New("open target database failed")
	}
	configureSQLConnectionPool(db)

	pingCtx, cancel := context.WithTimeout(ctx, dialTimeout)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		_ = db.Close()
		return errors.New("target database is unavailable")
	}
	m.conn = db
	m.pingTimeout = dialTimeout
	return nil
}

func (m *MySQLDB) Close() error {
	if m == nil || m.conn == nil {
		return nil
	}
	return m.conn.Close()
}

func (m *MySQLDB) Ping(ctx context.Context) error {
	if m == nil || m.conn == nil {
		return errors.New("connection is not open")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	timeout := m.pingTimeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	pingCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return m.conn.PingContext(pingCtx)
}

func (m *MySQLDB) SQLDB() *sql.DB {
	if m == nil {
		return nil
	}
	return m.conn
}
