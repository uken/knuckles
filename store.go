package knuckles

import (
	"fmt"
	"github.com/fiorix/go-redis/redis"
)

type Endpoint struct {
	addr string
}

type Store interface {
	EndpointForHostname(name string) (Endpoint, error)

	AddApplication(app string) error
	AddHostname(app, hostname string) error
	AddBackend(app, backend string, ttl int) error

	HostnamesForApp(app string) ([]string, error)
	BackendsForApp(app string) ([]string, error)

	RemoveApplication(name string) error
	RemoveHostname(app, hostname string) error
	RemoveBackend(app, backend string) error

	ListApplications() ([]string, error)
	DescribeApplication(app string) ([]string, []string, error)
}

type RedisStore struct {
	client    *redis.Client
	namespace string
}

func NewRedisStore(namespace string, hosts []string) (*RedisStore, error) {
	r := &RedisStore{
		namespace: namespace,
	}

	r.client = redis.New(hosts...)

	return r, nil
}

func (r *RedisStore) EndpointForHostname(name string) (Endpoint, error) {
	var epoint Endpoint
	appName, err := r.client.Get(r.Key("resolve:%s", name))

	if err != nil {
		return epoint, err
	}

	if appName == "" {
		return epoint, ErrNoHostname
	}

	members, err := r.client.SRandMember(r.Key("backend:%s", appName), 1)

	if err != nil {
		return epoint, err
	}

	if len(members) != 1 {
		return epoint, ErrNoBackend
	}

	backend := members[0]

	//check if ttl is still valid
	exists, err := r.client.Exists(r.Key("backend_ttl:%s:%s", appName, backend))

	if err != nil {
		return epoint, err
	}

	if !exists {
		return epoint, ErrDeadBackend
	}

	epoint.addr = backend

	return epoint, nil
}

func (r *RedisStore) Key(format string, args ...interface{}) string {
	return r.namespace + fmt.Sprintf(format, args...)
}

func (r *RedisStore) AddApplication(app string) error {
	ok, err := r.client.SAdd(r.Key("apps"), app)

	if err != nil {
		return err
	}

	if ok == 0 {
		return ErrAppAlreadyExists
	}

	return nil
}

func (r *RedisStore) isValidApp(app string) error {
	ismember, err := r.client.SIsMember(r.Key("apps"), app)
	if err != nil {
		return err
	}
	if ismember == 0 {
		return ErrNoApp
	}

	return nil
}

func (r *RedisStore) AddHostname(app, hostname string) error {
	err := r.isValidApp(app)
	if err != nil {
		return err
	}

	ok, err := r.client.SetNx(r.Key("resolve:%s", hostname), app)

	if err != nil {
		return err
	}

	if ok == 0 {
		return ErrHostnameAlreadyExists
	}

	_, err = r.client.SAdd(r.Key("hostname:%s", app), hostname)

	return err
}

func (r *RedisStore) AddBackend(app, backend string, ttl int) error {
	err := r.isValidApp(app)
	if err != nil {
		return err
	}

	ismember, err := r.client.SIsMember(r.Key("backends:%s", app), backend)

	if err != nil {
		return err
	}

	if ismember == 1 {
		return ErrBackendAlreadyExists
	}

	err = r.client.Set(r.Key("backend_ttl:%s:%s", app, backend), "1")

	if err != nil {
		return err
	}

	if ttl > 0 {
		_, err = r.client.Expire(r.Key("backend_ttl:%s:%s", app, backend), ttl)

		if err != nil {
			return err
		}
	}

	_, err = r.client.SAdd(r.Key("backend:%s", app), backend)

	return err
}

func (r *RedisStore) HostnamesForApp(app string) ([]string, error) {
	return r.client.SMembers(r.Key("hostname:%s", app))
}

func (r *RedisStore) BackendsForApp(app string) ([]string, error) {
	return r.client.SMembers(r.Key("backend:%s", app))
}

func (r *RedisStore) RemoveBackend(app, backend string) error {
	err := r.isValidApp(app)
	if err != nil {
		return err
	}

	_, err = r.client.SRem(r.Key("backend:%s", app), backend)
	return err
}

func (r *RedisStore) RemoveHostname(app, hostname string) error {
	err := r.isValidApp(app)
	if err != nil {
		return err
	}

	_, err = r.client.SRem(r.Key("hostname:%s", app), hostname)

	if err != nil {
		return err
	}

	_, err = r.client.Del(r.Key("resolve:%s", hostname))
	return err
}

func (r *RedisStore) RemoveApplication(app string) error {
	err := r.isValidApp(app)
	if err != nil {
		return err
	}
	_, err = r.client.Del(r.Key("backend:%s", app))
	if err != nil {
		return err
	}

	hostnames, err := r.HostnamesForApp(app)
	for _, h := range hostnames {
		_, err = r.client.Del(r.Key("resolve:%s", h))
		if err != nil {
			return err
		}
	}

	_, err = r.client.Del(r.Key("hostname:%s", app))
	if err != nil {
		return err
	}

	_, err = r.client.SRem(r.Key("apps"), app)
	return err
}

func (r *RedisStore) ListApplications() ([]string, error) {
	return r.client.SMembers(r.Key("apps"))
}

func (r *RedisStore) DescribeApplication(app string) ([]string, []string, error) {
	var hostnames []string
	var backends []string

	err := r.isValidApp(app)
	if err != nil {
		return hostnames, backends, err
	}

	hostnames, err = r.client.SMembers(r.Key("hostname:%s", app))
	if err != nil {
		return hostnames, backends, err
	}

	backends, err = r.client.SMembers(r.Key("backend:%s", app))
	return hostnames, backends, err
}

func (epoint *Endpoint) Addr() string {
	return epoint.addr
}
