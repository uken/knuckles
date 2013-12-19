package knuckles

import (
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"sync"
	"time"
)

type Frontend struct {
	mutex         sync.Mutex
	Backends      map[string]string
	keepAliveMap  map[string]bool
	LiveBackends  []string
	quitChan      chan bool
	CheckInterval time.Duration
	Name          string
}

func NewFrontend(name string, checkInterval time.Duration) *Frontend {
	fe := new(Frontend)
	fe.Name = name
	fe.CheckInterval = checkInterval
	fe.Backends = make(map[string]string)
	fe.keepAliveMap = make(map[string]bool)
	fe.LiveBackends = make([]string, 0)
	fe.quitChan = make(chan bool)

	return fe
}

func (self *Frontend) Start() {

	self.keepAlive()

	run := true
	for run {
		select {
		case <-self.quitChan:
			run = false
		case <-time.After(self.CheckInterval):
			self.keepAlive()
		}
	}

	self.quitChan <- true
}

func (self *Frontend) Stop() {
	log.Println("Stopping frontend", self.Name)
	self.quitChan <- true
	<-self.quitChan
	log.Println("Stopped frontend", self.Name)

	close(self.quitChan)
}

func (self *Frontend) keepAlive() {
	// rebuild list of active servers
	live := make([]string, 0)
	changed := false

	for name, endpoint := range self.Backends {
		previous := self.keepAliveMap[name]
		current := checkEndpoint(endpoint)
		if previous != current {
			self.keepAliveMap[name] = current
			changed = true
			log.Println("[", self.Name, "] Backend ", name, " IsAlive ", current)
		}
	}

	if changed {

		for name, alive := range self.keepAliveMap {
			if alive {
				live = append(live, self.Backends[name])
			}
		}

		self.mutex.Lock()
		self.LiveBackends = live
		self.mutex.Unlock()
	}
}

func (self *Frontend) PickBackend() (string, error) {
	self.mutex.Lock()
	defer self.mutex.Unlock()

	tot := len(self.LiveBackends)
	if tot < 1 {
		return "", fmt.Errorf("No backends available")
	}

	backend := self.LiveBackends[rand.Int()%tot]

	if backend == "" {
		return "", fmt.Errorf("No backends available")
	}

	return backend, nil
}

func (self *Frontend) AddBackend(name, endpoint string) error {
	self.mutex.Lock()
	defer self.mutex.Unlock()

	_, ok := self.Backends[name]
	if ok {
		return fmt.Errorf("Backend already present")
	}

	self.Backends[name] = endpoint

	return nil
}

func checkEndpoint(endpoint string) bool {
	tr := &http.Transport{}

	req, _ := http.NewRequest("GET", fmt.Sprintf("http://%s", endpoint), nil)
	req.Close = true

	resp, err := tr.RoundTrip(req)
	if err == nil {
		defer resp.Body.Close()
	}

	if err != nil {
		return false
	}

	return true
}
