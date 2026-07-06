package mysql

import (
	"net"
	"strconv"

	mysqldriver "github.com/go-sql-driver/mysql"
)

func buildDSN(endpoint compiledEndpoint, database string, params map[string]string) string {
	port := endpoint.Port
	if port == 0 {
		port = defaultPort
	}
	cfg := mysqldriver.NewConfig()
	cfg.User = endpoint.Username
	cfg.Passwd = endpoint.Password
	cfg.Net = mysqlNetwork
	cfg.Addr = net.JoinHostPort(endpoint.Host, strconv.Itoa(port))
	cfg.DBName = database
	cfg.Params = cloneParams(params)
	return cfg.FormatDSN()
}

func mergeParams(values ...map[string]string) map[string]string {
	var out map[string]string
	for _, value := range values {
		if len(value) == 0 {
			continue
		}
		if out == nil {
			out = make(map[string]string)
		}
		for key, item := range value {
			out[key] = item
		}
	}
	return out
}

func cloneParams(value map[string]string) map[string]string {
	if len(value) == 0 {
		return nil
	}
	out := make(map[string]string, len(value))
	for key, item := range value {
		out[key] = item
	}
	return out
}

func mergePool(base PoolConfig, override PoolConfig) PoolConfig {
	out := base
	if override.MaxOpenConns != 0 {
		out.MaxOpenConns = override.MaxOpenConns
	}
	if override.MaxIdleConns != 0 {
		out.MaxIdleConns = override.MaxIdleConns
	}
	if override.ConnMaxLifetime != "" {
		out.ConnMaxLifetime = override.ConnMaxLifetime
	}
	if override.ConnMaxIdleTime != "" {
		out.ConnMaxIdleTime = override.ConnMaxIdleTime
	}
	return out
}
