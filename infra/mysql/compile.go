package mysql

import (
	"fmt"
	"sort"
	"strings"
)

// CompiledConfig is the runtime-ready MySQL configuration.
type CompiledConfig struct {
	Instances map[string]CompiledInstance
	Aliases   map[string]string
}

// CompiledInstance describes one resolved MySQL database instance.
type CompiledInstance struct {
	Name      string
	Server    string
	Database  string
	WriteDSN  string
	ServerDSN string
	ReadDSNs  []string
	Pool      PoolConfig
	ReadPool  PoolConfig
	Ensure    EnsureDatabaseConfig
}

type compiledEndpoint struct {
	Host     string
	Port     int
	Username string
	Password string
	Params   map[string]string
}

type serverCompileInput struct {
	name       string
	path       string
	server     ServerConfig
	params     map[string]string
	pool       PoolConfig
	readPool   PoolConfig
	multi      bool
	registerAs func(logical string, instance CompiledInstance) error
}

// Compile validates and compiles structured MySQL config into runtime instances.
func (c Config) Compile() (CompiledConfig, error) {
	out := CompiledConfig{
		Instances: map[string]CompiledInstance{},
		Aliases:   map[string]string{},
	}
	if c.IsZero() {
		return out, nil
	}
	if len(c.Servers) > 0 {
		if c.hasSingleServerFields() {
			return out, fmt.Errorf("mysql cannot mix servers with top-level host/write/reads/databases")
		}
		for _, serverName := range sortedServerNames(c.Servers) {
			server := c.Servers[serverName]
			if err := validateKey("mysql server", serverName, false); err != nil {
				return out, err
			}
			err := compileServer(serverCompileInput{
				name:       serverName,
				path:       "mysql.servers." + serverName,
				server:     server,
				params:     cloneParams(c.Params),
				pool:       c.Pool,
				readPool:   c.ReadPool,
				multi:      true,
				registerAs: out.register,
			})
			if err != nil {
				return out, err
			}
		}
		return out, nil
	}

	server := ServerConfig{
		Host:      c.Host,
		Port:      c.Port,
		Username:  c.Username,
		Password:  c.Password,
		Params:    cloneParams(c.Params),
		Write:     c.Write,
		Reads:     append([]EndpointConfig(nil), c.Reads...),
		Databases: cloneDatabases(c.Databases),
		Pool:      c.Pool,
		ReadPool:  c.ReadPool,
	}
	err := compileServer(serverCompileInput{
		name:       anonymousSingleServerID,
		path:       "mysql",
		server:     server,
		params:     nil,
		pool:       PoolConfig{},
		readPool:   PoolConfig{},
		multi:      false,
		registerAs: out.register,
	})
	if err != nil {
		return out, err
	}
	return out, nil
}

func (c Config) hasSingleServerFields() bool {
	return strings.TrimSpace(c.Host) != "" ||
		c.Port != 0 ||
		strings.TrimSpace(c.Username) != "" ||
		c.Password != "" ||
		!c.Write.IsZero() ||
		len(c.Reads) > 0 ||
		len(c.Databases) > 0
}

// Resolve resolves a MySQL runtime instance. Empty name selects "default".
func (c CompiledConfig) Resolve(name ...string) (CompiledInstance, error) {
	requested := defaultInstanceName
	if len(name) > 0 && strings.TrimSpace(name[0]) != "" {
		requested = strings.TrimSpace(name[0])
	}
	if len(c.Instances) == 0 {
		if requested != defaultInstanceName {
			return CompiledInstance{}, fmt.Errorf("mysql instance %q is not configured", requested)
		}
		return CompiledInstance{}, fmt.Errorf("mysql is not configured")
	}
	if instance, ok := c.Instances[requested]; ok {
		return instance, nil
	}
	if target, ok := c.Aliases[requested]; ok {
		if target == "" {
			return CompiledInstance{}, fmt.Errorf("mysql instance %q is ambiguous; use a qualified server.database name", requested)
		}
		if instance, ok := c.Instances[target]; ok {
			return instance, nil
		}
	}
	if requested == defaultInstanceName && len(c.Instances) == 1 {
		for _, instance := range c.Instances {
			return instance, nil
		}
	}
	return CompiledInstance{}, fmt.Errorf("mysql instance %q is not configured", requested)
}

func (c CompiledConfig) register(logical string, instance CompiledInstance) error {
	if _, exists := c.Instances[instance.Name]; exists {
		return fmt.Errorf("mysql instance %q is duplicated", instance.Name)
	}
	c.Instances[instance.Name] = instance
	if logical == instance.Name {
		return nil
	}
	if existing, ok := c.Aliases[logical]; ok {
		if existing != instance.Name {
			c.Aliases[logical] = ""
		}
		return nil
	}
	c.Aliases[logical] = instance.Name
	return nil
}

func compileServer(in serverCompileInput) error {
	if in.server.IsZero() {
		return fmt.Errorf("%s is empty", in.path)
	}
	if len(in.server.Databases) == 0 {
		return fmt.Errorf("%s.databases is required", in.path)
	}
	baseEndpoint := compiledEndpoint{
		Host:     strings.TrimSpace(in.server.Host),
		Port:     in.server.Port,
		Username: strings.TrimSpace(in.server.Username),
		Password: in.server.Password,
		Params:   mergeParams(in.params, in.server.Params),
	}
	writeEndpoint := baseEndpoint.merge(in.server.Write)
	if err := writeEndpoint.validate(in.path + ".write"); err != nil {
		return err
	}
	readEndpoints := make([]compiledEndpoint, 0, len(in.server.Reads))
	for i, read := range in.server.Reads {
		endpoint := baseEndpoint.merge(read)
		if err := endpoint.validate(fmt.Sprintf("%s.reads[%d]", in.path, i)); err != nil {
			return err
		}
		readEndpoints = append(readEndpoints, endpoint)
	}

	serverPool := mergePool(in.pool, in.server.Pool)
	serverReadPool := mergePool(in.readPool, in.server.ReadPool)
	if err := serverPool.Validate(in.path + ".pool"); err != nil {
		return err
	}
	if len(readEndpoints) > 0 || !serverReadPool.IsZero() {
		if err := serverReadPool.Validate(in.path + ".read_pool"); err != nil {
			return err
		}
	}

	for _, logical := range sortedDatabaseNames(in.server.Databases) {
		if err := validateKey(in.path+".databases", logical, false); err != nil {
			return err
		}
		database := in.server.Databases[logical]
		realName := strings.TrimSpace(database.Name)
		if realName == "" {
			realName = logical
		}
		ensure := database.Ensure.Normalize()
		if ensure.Enabled {
			if err := validateIdentifier(realName); err != nil {
				return fmt.Errorf("%s.databases.%s.name: %w", in.path, logical, err)
			}
			if err := validateIdentifier(ensure.Charset); err != nil {
				return fmt.Errorf("%s.databases.%s.ensure.charset: %w", in.path, logical, err)
			}
			if err := validateIdentifier(ensure.Collation); err != nil {
				return fmt.Errorf("%s.databases.%s.ensure.collation: %w", in.path, logical, err)
			}
		}

		instanceName := logical
		if in.multi {
			instanceName = in.name + instanceNameSeparator + logical
		}
		params := mergeParams(writeEndpoint.Params, database.Params)
		writeDSN := buildDSN(writeEndpoint, realName, params)
		serverDSN := buildDSN(writeEndpoint, "", writeEndpoint.Params)
		readDSNs := make([]string, 0, len(readEndpoints))
		for _, endpoint := range readEndpoints {
			readDSNs = append(readDSNs, buildDSN(endpoint, realName, mergeParams(endpoint.Params, database.Params)))
		}
		instance := CompiledInstance{
			Name:      instanceName,
			Server:    in.name,
			Database:  realName,
			WriteDSN:  writeDSN,
			ServerDSN: serverDSN,
			ReadDSNs:  readDSNs,
			Pool:      mergePool(serverPool, database.Pool).Normalize(),
			ReadPool:  serverReadPool.Normalize(),
			Ensure:    ensure,
		}
		if err := instance.Pool.Validate(in.path + ".databases." + logical + ".pool"); err != nil {
			return err
		}
		if len(readDSNs) > 0 {
			if err := instance.ReadPool.Validate(in.path + ".read_pool"); err != nil {
				return err
			}
		}
		if err := in.registerAs(logical, instance); err != nil {
			return err
		}
	}
	return nil
}

func (e compiledEndpoint) merge(override EndpointConfig) compiledEndpoint {
	out := e
	if strings.TrimSpace(override.Host) != "" {
		out.Host = strings.TrimSpace(override.Host)
	}
	if override.Port != 0 {
		out.Port = override.Port
	}
	if strings.TrimSpace(override.Username) != "" {
		out.Username = strings.TrimSpace(override.Username)
	}
	if override.Password != "" {
		out.Password = override.Password
	}
	out.Params = mergeParams(out.Params, override.Params)
	return out
}

func (e compiledEndpoint) validate(path string) error {
	if strings.TrimSpace(e.Host) == "" {
		return fmt.Errorf("%s.host is required", path)
	}
	if strings.TrimSpace(e.Username) == "" {
		return fmt.Errorf("%s.username is required", path)
	}
	if e.Port < 0 {
		return fmt.Errorf("%s.port must not be negative", path)
	}
	return nil
}

func validateKey(path string, value string, allowDot bool) error {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fmt.Errorf("%s name is required", path)
	}
	if trimmed != value {
		return fmt.Errorf("%s name %q must not contain surrounding whitespace", path, value)
	}
	if !allowDot && strings.Contains(value, instanceNameSeparator) {
		return fmt.Errorf("%s name %q must not contain %q", path, value, instanceNameSeparator)
	}
	return nil
}

func sortedServerNames(values map[string]ServerConfig) []string {
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func sortedDatabaseNames(values map[string]DatabaseConfig) []string {
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func cloneDatabases(values map[string]DatabaseConfig) map[string]DatabaseConfig {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]DatabaseConfig, len(values))
	for name, value := range values {
		out[name] = value
	}
	return out
}
