package knuckles

import (
  "github.com/fiorix/go-redis/redis"
  "testing"
)

var namespace = "test:"
var addr = "localhost:6379"

func redisClear() {
  c := redis.New(addr)
  c.FlushAll()
}

func Test_StoreBasic(t *testing.T) {
  redisClear()
  r, err := NewRedisStore(namespace, addr)

  if err != nil {
    t.Fatal(err)
  }

  err = r.AddApplication("testapp")

  if err != nil {
    t.Fatal(err)
  }
}

func Test_StoreHostname(t *testing.T) {
  redisClear()
  r, err := NewRedisStore(namespace, addr)

  if err != nil {
    t.Fatal(err)
  }

  err = r.AddApplication("testapp")

  if err != nil {
    t.Fatal(err)
  }

  err = r.AddHostname("testapp", "something.com")

  if err != nil {
    t.Fatal(err)
  }

  err = r.AddHostname("testapp", "something.com")

  if err != ErrHostnameAlreadyExists {
    t.Fatal("Duplicated hostname")
  }
}

func Test_StoreBackend(t *testing.T) {
  redisClear()
  r, err := NewRedisStore(namespace, addr)

  if err != nil {
    t.Fatal(err)
  }

  err = r.AddApplication("testapp")

  if err != nil {
    t.Fatal(err)
  }

  r.AddHostname("testapp", "something.com")

  err = r.AddBackend("testapp", "something.com:8080", 10)

  if err != nil {
    t.Fatal(err)
  }

  err = r.AddBackend("testapp", "something.com:8080", 10)

  if err != nil {
    t.Fatal(err)
  }

  bk, err := r.EndpointForHostname("something.com")

  if err != nil {
    t.Fatal(err)
  }

  if bk.Addr() != "something.com:8080" {
    t.Fatal("Invalid backend", bk)
  }
}
