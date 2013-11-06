package knuckles

import (
	"fmt"
	"log"
	"net"
	"sync"
	"time"
)

type backendStat struct {
	Frontend   string
	Backend    string
	StatusCode int
}

type statsHolder struct {
	Reloads    uint64
	Requests   uint64
	NoHostname uint64
	NoBackend  uint64
	Error      uint64
	Frontends  map[string]uint64
	// Backends[frontend][backend][status] = counter
	Backends map[backendStat]uint64
}

type HTTPStats struct {
	mtx      sync.Mutex
	stats    *statsHolder
	statsD   net.Conn
	quitChan chan bool
	prefix   string
}

type Metric int

const (
	MetricRequest Metric = iota
	MetricNoHostname
	MetricNoBackend
	MetricError
	MetricReload
)

func newstatsHolder() *statsHolder {
	ret := new(statsHolder)
	ret.Frontends = make(map[string]uint64)
	ret.Backends = make(map[backendStat]uint64)
	return ret
}

func NewHTTPStats(endpoint, prefix string) (*HTTPStats, error) {
	var err error

	ret := new(HTTPStats)
	ret.quitChan = make(chan bool)
	ret.stats = newstatsHolder()
	ret.prefix = prefix
	ret.statsD, err = net.Dial("udp", endpoint)

	return ret, err
}

func (self *HTTPStats) Start() {
	run := true
	for run {
		select {
		case <-self.quitChan:
			run = false
		case <-time.After(1 * time.Second):
			self.dispatch()
		}
	}
	self.quitChan <- true
}

func (self *HTTPStats) dispatch() {
	self.mtx.Lock()
	oldStats := self.stats
	self.stats = newstatsHolder()
	self.mtx.Unlock()

	self.sendStat("_.requests", oldStats.Requests)
	self.sendStat("_.no_backend", oldStats.NoBackend)
	self.sendStat("_.no_hostname", oldStats.NoHostname)
	self.sendStat("_.errors", oldStats.Error)

	for k, v := range oldStats.Backends {
		self.sendStat(k.String(), v)
	}
}

func (self *HTTPStats) Stop() {
	self.quitChan <- true
	<-self.quitChan
	self.statsD.Close()
	close(self.quitChan)
}

func (self *HTTPStats) Increment(metric Metric) {
	self.mtx.Lock()
	defer self.mtx.Unlock()
	switch metric {
	case MetricRequest:
		self.stats.Requests += 1
	case MetricNoHostname:
		self.stats.NoHostname += 1
	case MetricNoBackend:
		self.stats.NoBackend += 1
	case MetricError:
		self.stats.Error += 1
	case MetricReload:
		self.stats.Reloads += 1
	}
}

func (self *HTTPStats) IncrementFrontend(name string) {
	self.mtx.Lock()
	defer self.mtx.Unlock()
	self.stats.Frontends[name] += 1
}

func (self *HTTPStats) IncrementBackend(frontend, backend string, status int) {
	self.mtx.Lock()
	defer self.mtx.Unlock()
	statKey := backendStat{
		Frontend:   frontend,
		Backend:    backend,
		StatusCode: status,
	}
	self.stats.Backends[statKey] += 1
}

func (self *HTTPStats) statName(name string) string {
	return fmt.Sprintf("%s%s", self.prefix, name)
}

func (self *HTTPStats) sendStat(name string, value uint64) {
	// don't bother publishing if valus is zero (this is a counter, not a gauge)
	if value < 1 {
		return
	}

	_, err := fmt.Fprintf(self.statsD, "%s%s:%d|c", self.prefix, name, value)
	if err != nil {
		log.Println(err)
	}
}

func (self *backendStat) String() string {
	return fmt.Sprintf("%s.%s.%d", self.Frontend, self.Backend, self.StatusCode)
}
