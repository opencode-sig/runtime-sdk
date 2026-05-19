package servicekit

import (
	"testing"

	infraredis "github.com/opencode-sig/runtime-sdk/infra/redis"
)

func TestInfraContainerReusesLazyClients(t *testing.T) {
	container := NewInfraContainer(InfraConfig{
		Redis: testRedisConfig(),
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
}

func TestInfraContainerRejectsNamedInstanceWithoutConfig(t *testing.T) {
	container := NewInfraContainer(InfraConfig{Redis: testRedisConfig()})
	if _, err := container.Redis("analytics"); err == nil {
		t.Fatal("expected named infra error")
	}
}

func TestInfraContainerClosePreventsNewClients(t *testing.T) {
	container := NewInfraContainer(InfraConfig{Redis: testRedisConfig()})
	if err := container.Close(); err != nil {
		t.Fatalf("close infra: %v", err)
	}
	if _, err := container.Redis(); err == nil {
		t.Fatal("expected closed infra error")
	}
}

func testRedisConfig() infraredis.Config {
	return infraredis.Config{Addrs: []string{"127.0.0.1:6379"}}
}
