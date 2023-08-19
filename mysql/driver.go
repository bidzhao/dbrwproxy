package mysql

import (
	"context"
	"database/sql/driver"
	"net"
	"sync"
)

// MySQLDriver is exported to make the driver directly accessible.
// In general the driver is used via the database/sql package.
type MySQLDriver struct{}

// DialFunc is a function which can be used to establish the network connection.
// Custom dial functions must be registered with RegisterDial
//
// Deprecated: users should register a DialContextFunc instead
type DialFunc func(addr string) (net.Conn, error)

// DialContextFunc is a function which can be used to establish the network connection.
// Custom dial functions must be registered with RegisterDialContext
type DialContextFunc func(ctx context.Context, addr string) (net.Conn, error)

var (
	dialsLock sync.RWMutex
	dials     map[string]DialContextFunc
)

// RegisterDialContext registers a custom dial function. It can then be used by the
// network address mynet(addr), where mynet is the registered new network.
// The current context for the connection and its address is passed to the dial function.
func RegisterDialContext(net string, dial DialContextFunc) {
	dialsLock.Lock()
	defer dialsLock.Unlock()
	if dials == nil {
		dials = make(map[string]DialContextFunc)
	}
	dials[net] = dial
}

// DeregisterDialContext removes the custom dial function registered with the given net.
func DeregisterDialContext(net string) {
	dialsLock.Lock()
	defer dialsLock.Unlock()
	if dials != nil {
		delete(dials, net)
	}
}

// RegisterDial registers a custom dial function. It can then be used by the
// network address mynet(addr), where mynet is the registered new network.
// addr is passed as a parameter to the dial function.
//
// Deprecated: users should call RegisterDialContext instead
func RegisterDial(network string, dial DialFunc) {
	RegisterDialContext(network, func(_ context.Context, addr string) (net.Conn, error) {
		return dial(addr)
	})
}

// Open new Connection.
// See https://github.com/go-sql-driver/mysql#dsn-data-source-name for how
// the DSN string is formatted
func (d MySQLDriver) Open(dsn string) (driver.Conn, error) {
	cfg, err := ParseDSN(dsn)
	if err != nil {
		return nil, err
	}
	c, err := newConnector(cfg)
	if err != nil {
		return nil, err
	}
	return c.Connect(context.Background())
}

func CreateConnector(dsn string) (*Connector, error) {
	cfg, err := ParseDSN(dsn)
	if err != nil {
		return nil, err
	}
	return newConnector(cfg)
}

// NewConnector returns new driver.Connector.
func NewConnector(cfg *Config) (driver.Connector, error) {
	cfg = cfg.Clone()
	// normalize the contents of cfg so calls to NewConnector have the same
	// behavior as MySQLDriver.OpenConnector
	if err := cfg.normalize(); err != nil {
		return nil, err
	}
	return newConnector(cfg)
}

// OpenConnector implements driver.DriverContext.
func (d MySQLDriver) OpenConnector(dsn string) (driver.Connector, error) {
	cfg, err := ParseDSN(dsn)
	if err != nil {
		return nil, err
	}
	return newConnector(cfg)
}
