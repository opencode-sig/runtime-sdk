package control

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"strings"
	"sync"
	"time"

	"go.etcd.io/etcd/api/v3/mvccpb"
	clientv3 "go.etcd.io/etcd/client/v3"
)

const (
	CommandRebuild = "rebuild"
	CommandRestart = "restart"
)

// Command is a runtime control-plane command.
type Command struct {
	Command    string    `json:"command" yaml:"command"`
	Service    string    `json:"service" yaml:"service"`
	InstanceID string    `json:"instance_id,omitempty" yaml:"instance_id,omitempty"`
	Reason     string    `json:"reason,omitempty" yaml:"reason,omitempty"`
	CreatedAt  time.Time `json:"created_at" yaml:"created_at"`
}

// Store publishes and watches runtime commands.
type Store interface {
	Publish(ctx context.Context, command Command) error
	Watch(ctx context.Context, service string) (<-chan Command, error)
}

// EtcdStore stores runtime commands in etcd.
type EtcdStore struct {
	client *clientv3.Client
	prefix string
}

// NewEtcdStore creates an etcd-backed command store.
func NewEtcdStore(client *clientv3.Client, prefix string) *EtcdStore {
	return &EtcdStore{client: client, prefix: cleanPrefix(prefix)}
}

// Publish writes one command under the target service command prefix.
func (s *EtcdStore) Publish(ctx context.Context, command Command) error {
	if s == nil || s.client == nil {
		return fmt.Errorf("control etcd client is required")
	}
	command.Command = strings.TrimSpace(command.Command)
	command.Service = strings.TrimSpace(command.Service)
	if command.Command == "" {
		return fmt.Errorf("control command is required")
	}
	if command.Service == "" {
		return fmt.Errorf("control command service is required")
	}
	if command.CreatedAt.IsZero() {
		command.CreatedAt = time.Now().UTC()
	}
	data, err := json.Marshal(command)
	if err != nil {
		return err
	}
	key := path.Join(s.prefix, command.Service, fmt.Sprintf("%d", command.CreatedAt.UnixNano()))
	_, err = s.client.Put(ctx, key, string(data))
	return err
}

// Watch watches service-specific commands and global all-service commands.
func (s *EtcdStore) Watch(ctx context.Context, service string) (<-chan Command, error) {
	if s == nil || s.client == nil {
		return nil, fmt.Errorf("control etcd client is required")
	}
	service = strings.TrimSpace(service)
	if service == "" {
		return nil, fmt.Errorf("control watch service is required")
	}
	out := make(chan Command, 16)
	watches := []clientv3.WatchChan{s.watch(ctx, service)}
	if service != "all" {
		watches = append(watches, s.watch(ctx, "all"))
	}

	go func() {
		defer close(out)
		var wg sync.WaitGroup
		commands := make(chan Command, 16)
		for _, watch := range watches {
			wg.Add(1)
			go func(watch clientv3.WatchChan) {
				defer wg.Done()
				for resp := range watch {
					if resp.Err() != nil {
						return
					}
					for _, event := range resp.Events {
						command, ok := commandFromEvent(event)
						if !ok {
							continue
						}
						select {
						case commands <- command:
						case <-ctx.Done():
							return
						}
					}
				}
			}(watch)
		}

		go func() {
			wg.Wait()
			close(commands)
		}()

		for {
			select {
			case command, ok := <-commands:
				if !ok {
					return
				}
				select {
				case out <- command:
				case <-ctx.Done():
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, nil
}

func (s *EtcdStore) watch(ctx context.Context, service string) clientv3.WatchChan {
	return s.client.Watch(ctx, path.Join(s.prefix, service)+"/", clientv3.WithPrefix())
}

func commandFromEvent(event *clientv3.Event) (Command, bool) {
	if event == nil || event.Type != mvccpb.PUT || event.Kv == nil {
		return Command{}, false
	}
	var command Command
	if err := json.Unmarshal(event.Kv.Value, &command); err != nil {
		return Command{}, false
	}
	return command, true
}

func cleanPrefix(prefix string) string {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return "/runtime/control/commands"
	}
	return "/" + strings.Trim(prefix, "/")
}
