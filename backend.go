package knuckles

import (
	"fmt"
	"net/http"
	"net/url"
	"time"
)

type Backend struct {
	Name          string
	Endpoint      string
	CheckInterval time.Duration
	CheckURL      *url.URL
	Alive         bool
	quitChan      chan bool
	notifyChan    chan BackendStatus
}

type BackendStatus struct {
	Name  string
	Alive bool
}

type BackendSettings struct {
	Endpoint      string
	CheckUrl      string
	CheckInterval time.Duration
	Updates       chan BackendStatus
}

func NewBackend(name string, settings BackendSettings) (*Backend, error) {
	var err error
	be := new(Backend)
	be.Name = name
	be.Alive = false
	be.quitChan = make(chan bool)

	if settings.CheckInterval == 0 ||
		settings.CheckUrl == "" ||
		settings.Endpoint == "" ||
		settings.Updates == nil {
		return nil, fmt.Errorf("Missing parameters")
	}
	be.Endpoint = settings.Endpoint
	be.notifyChan = settings.Updates
	be.CheckInterval = settings.CheckInterval

	be.CheckURL, err = url.Parse(settings.CheckUrl)
	return be, err
}

func (self *Backend) Start() {
	run := true

	self.CheckHealth()
	for run {
		select {
		case <-self.quitChan:
			run = false
		case <-time.After(self.CheckInterval):
			self.CheckHealth()
		}
	}
	self.quitChan <- true
}

func (self *Backend) Stop() {
	self.quitChan <- true
	<-self.quitChan
	close(self.quitChan)
}

func (self *Backend) CheckHealth() {
	current := self.Alive

	resp, err := http.Get(self.CheckURL.String())
	if err != nil || (resp.StatusCode >= 400) {
		self.Alive = false
	} else {
		self.Alive = true
	}

	if current != self.Alive {
		status := BackendStatus{
			Name:  self.Name,
			Alive: self.Alive,
		}
		self.notifyChan <- status
	}
}
