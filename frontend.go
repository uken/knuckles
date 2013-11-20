package knuckles

import (
	"fmt"
	"log"
	"math/rand"
)

type Frontend struct {
	Name         string
	Backends     map[string]*Backend
	LiveBackends []*Backend
	NotifyChan   chan BackendStatus
	quitChan     chan bool
}

func NewFrontend(name string) *Frontend {
	fe := new(Frontend)
	fe.Name = name
	fe.Backends = make(map[string]*Backend)
	fe.LiveBackends = make([]*Backend, 0)
	fe.NotifyChan = make(chan BackendStatus)
	fe.quitChan = make(chan bool)

	return fe
}

func (self *Frontend) Start() {
	run := true
	for run {
		select {
		case <-self.quitChan:
			run = false
		case status := <-self.NotifyChan:
			self.handleStatus(status)
		}
	}

	self.quitChan <- true
}

func (self *Frontend) Stop() {
	self.quitChan <- true
	<-self.quitChan

	close(self.quitChan)
	close(self.NotifyChan)

	// also stop all backends
	for _, backend := range self.Backends {
		backend.Stop()
	}
}
func (self *Frontend) handleStatus(status BackendStatus) {
	log.Println("Frontend:", self.Name, "Backend:", status.Name, "Alive:", status.Alive)

	// rebuild list of active servers
	live := make([]*Backend, 0)
	for _, backend := range self.Backends {
		if backend.Alive {
			live = append(live, backend)
		}
	}
	self.LiveBackends = live
}

func (self *Frontend) PickBackend() (*Backend, error) {
	tot := len(self.LiveBackends)
	if tot < 1 {
		return nil, fmt.Errorf("No backends available")
	}

	backend := self.LiveBackends[rand.Int()%tot]

	if backend == nil {
		return nil, fmt.Errorf("No backends available")
	}

	return backend, nil
}

func (self *Frontend) AddBackend(backend *Backend) error {
	_, ok := self.Backends[backend.Name]
	if ok {
		return fmt.Errorf("Backend already present")
	}

	self.Backends[backend.Name] = backend
	go backend.Start()
	return nil
}

func (self *Frontend) BackendStatus(name string) bool {

	be, ok := self.Backends[name]
	if ok {
		return be.Alive
	}

	return false
}
