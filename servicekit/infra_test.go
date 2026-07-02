package servicekit

import (
	"strings"
	"testing"

	infraelastic "github.com/opencode-sig/runtime-sdk/infra/elastic"
	inframinio "github.com/opencode-sig/runtime-sdk/infra/minio"
	infraredis "github.com/opencode-sig/runtime-sdk/infra/redis"
)

func TestInfraContainerReusesLazyClients(t *testing.T) {
	container := NewInfraContainer(InfraConfig{
		MySQL: MySQLConfigs{
			"default": testMySQLConfig("app"),
			"report":  testMySQLConfig("report"),
		},
		Redis:   testRedisConfig(),
		Elastic: testElasticConfig(),
		MinIO:   testMinIOConfig(),
	})
	defer func() {
		if err := container.Close(); err != nil {
			t.Fatalf("close infra: %v", err)
		}
	}()

	first, err := container.Redis()
	if err != nil {
		t.Fatalf("first redis: %v", err)
	}
	second, err := container.Redis()
	if err != nil {
		t.Fatalf("second redis: %v", err)
	}
	if first == nil || first != second {
		t.Fatalf("redis client was not reused: %#v %#v", first, second)
	}

	firstMySQL, err := container.MySQL()
	if err != nil {
		t.Fatalf("first mysql: %v", err)
	}
	secondMySQL, err := container.MySQL()
	if err != nil {
		t.Fatalf("second mysql: %v", err)
	}
	if firstMySQL == nil || firstMySQL != secondMySQL {
		t.Fatalf("mysql default client was not reused: %#v %#v", firstMySQL, secondMySQL)
	}
	reportMySQL, err := container.MySQL("report")
	if err != nil {
		t.Fatalf("report mysql: %v", err)
	}
	reportMySQLAgain, err := container.MySQL("report")
	if err != nil {
		t.Fatalf("report mysql again: %v", err)
	}
	if reportMySQL == nil || reportMySQL != reportMySQLAgain {
		t.Fatalf("mysql report client was not reused: %#v %#v", reportMySQL, reportMySQLAgain)
	}
	if reportMySQL == firstMySQL {
		t.Fatal("named mysql instance reused default client")
	}

	firstElastic, err := container.Elastic()
	if err != nil {
		t.Fatalf("first elastic: %v", err)
	}
	secondElastic, err := container.Elastic()
	if err != nil {
		t.Fatalf("second elastic: %v", err)
	}
	if firstElastic == nil || firstElastic != secondElastic {
		t.Fatalf("elastic client was not reused: %#v %#v", firstElastic, secondElastic)
	}

	firstMinIO, err := container.MinIO()
	if err != nil {
		t.Fatalf("first minio: %v", err)
	}
	secondMinIO, err := container.MinIO()
	if err != nil {
		t.Fatalf("second minio: %v", err)
	}
	if firstMinIO == nil || firstMinIO != secondMinIO {
		t.Fatalf("minio client was not reused: %#v %#v", firstMinIO, secondMinIO)
	}
}

func TestInfraContainerRejectsNamedInstanceWithoutConfig(t *testing.T) {
	container := NewInfraContainer(InfraConfig{Redis: testRedisConfig()})
	if _, err := container.MySQL("analytics"); err == nil || !strings.Contains(err.Error(), `mysql instance "analytics" is not configured`) {
		t.Fatalf("expected named mysql error, got %v", err)
	}
	if _, err := container.Redis("analytics"); err == nil {
		t.Fatal("expected named infra error")
	}
	if _, err := container.Elastic("search"); err == nil {
		t.Fatal("expected named elastic error")
	}
	if _, err := container.MinIO("assets"); err == nil {
		t.Fatal("expected named minio error")
	}
}

func TestInfraContainerClosePreventsNewClients(t *testing.T) {
	container := NewInfraContainer(InfraConfig{
		MySQL: MySQLConfigs{"default": testMySQLConfig("app")},
		Redis: testRedisConfig(),
	})
	if err := container.Close(); err != nil {
		t.Fatalf("close infra: %v", err)
	}
	if _, err := container.MySQL(); err == nil {
		t.Fatal("expected closed infra error")
	}
	if _, err := container.Redis(); err == nil {
		t.Fatal("expected closed infra error")
	}
}

func TestInfraContainerMySQLUsesSingleConfiguredInstanceAsDefault(t *testing.T) {
	container := NewInfraContainer(InfraConfig{
		MySQL: MySQLConfigs{"analytics": testMySQLConfig("analytics")},
	})
	defer func() {
		if err := container.Close(); err != nil {
			t.Fatalf("close infra: %v", err)
		}
	}()

	implicit, err := container.MySQL()
	if err != nil {
		t.Fatalf("implicit mysql: %v", err)
	}
	explicit, err := container.MySQL("analytics")
	if err != nil {
		t.Fatalf("explicit mysql: %v", err)
	}
	if implicit == nil || implicit != explicit {
		t.Fatalf("single mysql instance was not reused as default: %#v %#v", implicit, explicit)
	}
}

func testRedisConfig() infraredis.Config {
	return infraredis.Config{Addrs: []string{"127.0.0.1:6379"}}
}

func testElasticConfig() infraelastic.Config {
	return infraelastic.Config{Addresses: []string{"http://127.0.0.1:9200"}}
}

func testMinIOConfig() inframinio.Config {
	return inframinio.Config{
		Endpoint:  "127.0.0.1:9000",
		AccessKey: "minio",
		SecretKey: "secret",
	}
}
