package pool

import (
	"context"
	"dbrwproxy/mysql"
	"errors"
	"sync"
	"time"
)

// ConnectionPool manages a pool of database connections
type ConnectionPool struct {
	mu           sync.Mutex
	connector    *mysql.Connector
	conns        chan *mysql.MysqlConn
	minConns     int
	maxConns     int
	idleTimeout  time.Duration
	closed       bool
	closeChannel chan bool
}

// NewConnectionPool creates a new connection pool
func NewConnectionPool(connector *mysql.Connector, minConns, maxConns int, idleTimeout time.Duration) *ConnectionPool {
	return &ConnectionPool{
		conns:        make(chan *mysql.MysqlConn, maxConns),
		connector:    connector,
		minConns:     minConns,
		maxConns:     maxConns,
		idleTimeout:  idleTimeout,
		closeChannel: make(chan bool),
	}
}

// Open initializes the connection pool
func (cp *ConnectionPool) Open() error {
	cp.mu.Lock()
	defer cp.mu.Unlock()

	if cp.closed {
		return errors.New("connection pool is closed")
	}

	// initialize the specified minimum number of connections
	for i := 0; i < cp.minConns; i++ {
		conn, err := cp.connector.Connect(context.Background())
		if err != nil {
			return err
		}
		cp.conns <- conn.(*mysql.MysqlConn)
	}

	// start a goroutine to clean up idle connections
	go cp.reapIdleConns()

	return nil
}

// Get gets an available connection from the pool
func (cp *ConnectionPool) Get() (*mysql.MysqlConn, error) {
	cp.mu.Lock()

	if cp.closed {
		cp.mu.Unlock()
		return nil, errors.New("connection pool is closed")
	}

	// get connection from pool, or create a new one if pool is empty
	select {
	case db := <-cp.conns:
		cp.mu.Unlock()
		return db, nil
	default:
		conn, err := cp.connector.Connect(context.Background())
		cp.mu.Unlock()
		if err != nil {
			return nil, err
		}
		return conn.(*mysql.MysqlConn), nil
	}
}

// Put returns a connection to the pool
func (cp *ConnectionPool) Put(conn *mysql.MysqlConn) {
	cp.mu.Lock()
	defer cp.mu.Unlock()

	if cp.closed {
		_ = conn.Close()
		return
	}

	select {
	case cp.conns <- conn:
		// connection returned to pool
	default:
		// pool already full, close connection
		_ = conn.Close()
	}
}

// Close closes the connection pool
func (cp *ConnectionPool) Close() {
	cp.mu.Lock()
	defer cp.mu.Unlock()

	if cp.closed {
		return
	}

	cp.closed = true
	close(cp.closeChannel)

	for i := 0; i < len(cp.conns); i++ {
		conn := <-cp.conns
		_ = conn.Close()
	}
}

// reapIdleConns periodically closes idle connections
func (cp *ConnectionPool) reapIdleConns() {
	for {
		select {
		case <-cp.closeChannel:
			return
		case <-time.After(cp.idleTimeout):
			// close idle connections
			cp.mu.Lock()
			idleConns := len(cp.conns) - cp.minConns
			cp.mu.Unlock()
			for i := 0; i < idleConns; i++ {
				conn := <-cp.conns
				_ = conn.Close()
			}
		}
	}
}
