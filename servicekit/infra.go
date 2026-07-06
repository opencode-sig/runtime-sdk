package servicekit

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	infraelastic "github.com/opencode-sig/runtime-sdk/infra/elastic"
	infraetcd "github.com/opencode-sig/runtime-sdk/infra/etcd"
	infrakafka "github.com/opencode-sig/runtime-sdk/infra/kafka"
	inframinio "github.com/opencode-sig/runtime-sdk/infra/minio"
	inframysql "github.com/opencode-sig/runtime-sdk/infra/mysql"
	infraredis "github.com/opencode-sig/runtime-sdk/infra/redis"
	clientv3 "go.etcd.io/etcd/client/v3"
)

// Infra exposes lifecycle-managed infrastructure clients to service modules.
//
// Implementations decide whether clients are shared across modules in one
// DataPlane generation or isolated per process. Service code should not create
// long-lived infra clients directly.
type Infra interface {
	MySQL(name ...string) (*inframysql.DB, error)
	Redis(name ...string) (*infraredis.Client, error)
	KafkaProducer(name ...string) (*infrakafka.Producer, error)
	KafkaConsumer(topic string, groupID ...string) (*infrakafka.Consumer, error)
	Etcd(name ...string) (*clientv3.Client, error)
	Elastic(name ...string) (*infraelastic.Client, error)
	MinIO(name ...string) (*inframinio.Client, error)
}

// InfraContainer lazily creates and owns infra clients for one DataPlane generation.
type InfraContainer struct {
	cfg InfraConfig

	mu             sync.Mutex
	mysql          map[string]*inframysql.DB
	mysqlConfig    *inframysql.CompiledConfig
	redis          *infraredis.Client
	kafkaProducer  *infrakafka.Producer
	kafkaConsumers map[string]*infrakafka.Consumer
	etcd           *clientv3.Client
	elastic        *infraelastic.Client
	minio          *inframinio.Client
	closed         bool
}

// NewInfraContainer creates an infra client container for one DataPlane generation.
func NewInfraContainer(cfg InfraConfig) *InfraContainer {
	return &InfraContainer{
		mysql:          make(map[string]*inframysql.DB),
		cfg:            cfg,
		kafkaConsumers: make(map[string]*infrakafka.Consumer),
	}
}

// MySQL returns the named MySQL pools for this container.
func (c *InfraContainer) MySQL(name ...string) (*inframysql.DB, error) {
	if c == nil {
		return nil, fmt.Errorf("infra container is required")
	}
	c.mu.Lock()
	if err := c.ensureOpenLocked(); err != nil {
		c.mu.Unlock()
		return nil, err
	}
	instance, err := c.mysqlInstanceLocked(name...)
	if err != nil {
		c.mu.Unlock()
		return nil, err
	}
	if db := c.mysql[instance.Name]; db != nil {
		c.mu.Unlock()
		return db, nil
	}
	c.mu.Unlock()

	db, err := inframysql.NewDBFromCompiled(context.Background(), instance)
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.ensureOpenLocked(); err != nil {
		_ = db.Close()
		return nil, err
	}
	if existing := c.mysql[instance.Name]; existing != nil {
		_ = db.Close()
		return existing, nil
	}
	if c.mysql == nil {
		c.mysql = make(map[string]*inframysql.DB)
	}
	c.mysql[instance.Name] = db
	return db, nil
}

// Redis returns the shared Redis client for this container.
func (c *InfraContainer) Redis(name ...string) (*infraredis.Client, error) {
	if err := ensureDefaultInfraName(name...); err != nil {
		return nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.ensureOpenLocked(); err != nil {
		return nil, err
	}
	if c.redis != nil {
		return c.redis, nil
	}
	client, err := infraredis.NewClient(context.Background(), c.cfg.Redis)
	if err != nil {
		return nil, err
	}
	c.redis = client
	return client, nil
}

// KafkaProducer returns the shared Kafka producer for this container.
func (c *InfraContainer) KafkaProducer(name ...string) (*infrakafka.Producer, error) {
	if err := ensureDefaultInfraName(name...); err != nil {
		return nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.ensureOpenLocked(); err != nil {
		return nil, err
	}
	if c.kafkaProducer != nil {
		return c.kafkaProducer, nil
	}
	producer, err := infrakafka.NewProducer(c.cfg.Kafka)
	if err != nil {
		return nil, err
	}
	c.kafkaProducer = producer
	return producer, nil
}

// KafkaConsumer returns one shared Kafka consumer per topic/group pair.
func (c *InfraContainer) KafkaConsumer(topic string, groupID ...string) (*infrakafka.Consumer, error) {
	topic = strings.TrimSpace(topic)
	if topic == "" {
		return nil, fmt.Errorf("kafka consumer topic is required")
	}
	group := ""
	if len(groupID) > 0 {
		group = strings.TrimSpace(groupID[0])
	}
	key := topic + "\x00" + group

	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.ensureOpenLocked(); err != nil {
		return nil, err
	}
	if consumer := c.kafkaConsumers[key]; consumer != nil {
		return consumer, nil
	}
	consumer, err := infrakafka.NewConsumer(c.cfg.Kafka, topic, group)
	if err != nil {
		return nil, err
	}
	c.kafkaConsumers[key] = consumer
	return consumer, nil
}

// Etcd returns the shared etcd client for this container.
func (c *InfraContainer) Etcd(name ...string) (*clientv3.Client, error) {
	if err := ensureDefaultInfraName(name...); err != nil {
		return nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.ensureOpenLocked(); err != nil {
		return nil, err
	}
	if c.etcd != nil {
		return c.etcd, nil
	}
	client, err := infraetcd.NewClient(c.cfg.Etcd)
	if err != nil {
		return nil, err
	}
	c.etcd = client
	return client, nil
}

// Elastic returns the shared Elasticsearch client for this container.
func (c *InfraContainer) Elastic(name ...string) (*infraelastic.Client, error) {
	if err := ensureDefaultInfraName(name...); err != nil {
		return nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.ensureOpenLocked(); err != nil {
		return nil, err
	}
	if c.elastic != nil {
		return c.elastic, nil
	}
	client, err := infraelastic.NewClient(context.Background(), c.cfg.Elastic)
	if err != nil {
		return nil, err
	}
	c.elastic = client
	return client, nil
}

// MinIO returns the shared MinIO/S3-compatible client for this container.
func (c *InfraContainer) MinIO(name ...string) (*inframinio.Client, error) {
	if err := ensureDefaultInfraName(name...); err != nil {
		return nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.ensureOpenLocked(); err != nil {
		return nil, err
	}
	if c.minio != nil {
		return c.minio, nil
	}
	client, err := inframinio.NewClient(context.Background(), c.cfg.MinIO)
	if err != nil {
		return nil, err
	}
	c.minio = client
	return client, nil
}

// Close releases all created infra clients. It is safe to call multiple times.
func (c *InfraContainer) Close() error {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	mysql := make([]*inframysql.DB, 0, len(c.mysql))
	for _, db := range c.mysql {
		mysql = append(mysql, db)
	}
	redisClient := c.redis
	producer := c.kafkaProducer
	consumers := make([]*infrakafka.Consumer, 0, len(c.kafkaConsumers))
	for _, consumer := range c.kafkaConsumers {
		consumers = append(consumers, consumer)
	}
	etcdClient := c.etcd
	elasticClient := c.elastic
	minioClient := c.minio
	c.mu.Unlock()

	var err error
	for _, db := range mysql {
		if db != nil {
			err = errors.Join(err, db.Close())
		}
	}
	if redisClient != nil && redisClient.UniversalClient != nil {
		err = errors.Join(err, redisClient.Close())
	}
	if producer != nil {
		err = errors.Join(err, producer.Close())
	}
	for _, consumer := range consumers {
		if consumer != nil {
			err = errors.Join(err, consumer.Close())
		}
	}
	if etcdClient != nil {
		err = errors.Join(err, etcdClient.Close())
	}
	if elasticClient != nil {
		err = errors.Join(err, elasticClient.Close())
	}
	if minioClient != nil {
		err = errors.Join(err, minioClient.Close())
	}
	return err
}

func (c *InfraContainer) ensureOpenLocked() error {
	if c == nil {
		return fmt.Errorf("infra container is required")
	}
	if c.closed {
		return fmt.Errorf("infra container is closed")
	}
	return nil
}

func (c *InfraContainer) mysqlInstanceLocked(name ...string) (inframysql.CompiledInstance, error) {
	if c.mysqlConfig == nil {
		compiled, err := c.cfg.MySQL.Compile()
		if err != nil {
			return inframysql.CompiledInstance{}, err
		}
		c.mysqlConfig = &compiled
	}
	return c.mysqlConfig.Resolve(name...)
}

func ensureDefaultInfraName(name ...string) error {
	if len(name) == 0 || strings.TrimSpace(name[0]) == "" || strings.TrimSpace(name[0]) == "default" {
		return nil
	}
	return fmt.Errorf("named infra instance %q is not configured", strings.TrimSpace(name[0]))
}
