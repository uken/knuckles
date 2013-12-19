package knuckles

import (
	"time"
)

type DiscoveryListener interface {
	Load(duration time.Duration) (*HTTPConfig, error)
	Start() chan bool
	Stop()
	Config(hosts []string, namespace string) error
}

type HTTPConfig struct {
	HostMap   map[string]*Frontend
	Frontends []*Frontend
}

func NewHTTPConfig() *HTTPConfig {
	r := new(HTTPConfig)
	r.HostMap = make(map[string]*Frontend)
	r.Frontends = make([]*Frontend, 0)
	return r
}
