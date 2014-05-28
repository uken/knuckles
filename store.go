package knuckles

import (
  "fmt"
  "github.com/fiorix/go-redis/redis"
  "strconv"
  "time"
)

type Endpoint struct {
  addr string
}

type Store interface {
  EndpointForHostname(name string) (Endpoint, error)

  AddApplication(app string) error
  AddHostname(app, hostname string) error
  AddBackend(app, backend string, ttl int) error

  EnableBackend(app, backend string) error
  DisableBackend(app, backend string) error

  HostnamesForApp(app string) ([]string, error)
  BackendsForApp(app string) ([]string, error)

  RemoveApplication(name string) error
  RemoveHostname(app, hostname string) error
  RemoveBackend(app, backend string) error

  ListApplications() ([]string, error)
  DescribeApplication(app string) ([]string, map[string]bool, error)
}

type RedisStore struct {
  client    *redis.Client
  namespace string
}

func NewRedisStore(namespace string, host string) (*RedisStore, error) {
  r := &RedisStore{
    namespace: namespace,
  }

  r.client = redis.New(host)

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

  members, err := r.client.SRandMember(r.Key("live_backend:%s", appName), 1)

  if err != nil {
    return epoint, err
  }

  if len(members) != 1 {
    return epoint, ErrNoBackend
  }

  epoint.addr = members[0]

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

func (r *RedisStore) isValidBackend(app, backend string) error {
  ismember, err := r.client.SIsMember(r.Key("backend:%s", app), backend)
  if err != nil {
    return err
  }
  if ismember == 0 {
    return ErrNoBackend
  }

  // check if this guy needs to be removed (ttl)
  rawTTL, err := r.client.Get(r.Key("backend_ttl:%s:%s", app, backend))
  if err != nil {
    return err
  }

  // if the backend has been dead for more than X seconds
  // and it hasn't pinged us, get rid of it
  if rawTTL != "" {
    now := int(time.Now().Unix())
    ttl, err := strconv.Atoi(rawTTL)
    if err == nil {
      if now > ttl {
        err = r.RemoveBackend(app, backend)

        if err != nil {
          return err
        }
        //make sure this backend doesn't get activated
        return ErrNoBackend
      }
    }
  }

  return nil
}

func (r *RedisStore) EnableBackend(app, backend string) error {
  err := r.isValidApp(app)
  if err != nil {
    return err
  }

  err = r.isValidBackend(app, backend)
  if err != nil {
    return err
  }

  _, err = r.client.SAdd(r.Key("live_backend:%s", app), backend)

  return err
}

func (r *RedisStore) DisableBackend(app, backend string) error {
  err := r.isValidApp(app)
  if err != nil {
    return err
  }

  err = r.isValidBackend(app, backend)
  if err != nil {
    return err
  }

  _, err = r.client.SRem(r.Key("live_backend:%s", app), backend)

  return err
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

  if ttl > 0 {
    expire := int(time.Now().Unix()) + ttl
    err = r.client.Set(r.Key("backend_ttl:%s:%s", app, backend), strconv.Itoa(expire))

    if err != nil {
      return err
    }
  }

  _, err = r.client.SAdd(r.Key("backend:%s", app), backend)
  if err != nil {
    return err
  }

  return r.EnableBackend(app, backend)
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

  _, err = r.client.Del(r.Key("backend_ttl:%s:%s", app, backend))
  if err != nil {
    return err
  }

  _, err = r.client.SRem(r.Key("live_backend:%s", app), backend)
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

func (r *RedisStore) DescribeApplication(app string) ([]string, map[string]bool, error) {
  var hostnames []string
  var backends = make(map[string]bool)

  err := r.isValidApp(app)
  if err != nil {
    return hostnames, backends, err
  }

  hostnames, err = r.client.SMembers(r.Key("hostname:%s", app))
  if err != nil {
    return hostnames, backends, err
  }

  backendList, err := r.client.SMembers(r.Key("backend:%s", app))
  if err != nil {
    return hostnames, backends, err
  }

  for _, backend := range backendList {
    ok, err := r.client.SIsMember(r.Key("live_backend:%s", app), backend)
    if err != nil {
      return hostnames, backends, err
    }

    if ok > 0 {
      backends[backend] = true
    } else {
      backends[backend] = false
    }
  }

  return hostnames, backends, nil
}

func (epoint *Endpoint) Addr() string {
  return epoint.addr
}
