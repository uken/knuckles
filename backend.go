package knuckles

import (
	"fmt"
	"log"
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

	client := &http.Client{}

	req, _ := http.NewRequest("GET", self.CheckURL.String(), nil)
	req.Close = true

	resp, err := client.Do(req)
	if err == nil {
		defer resp.Body.Close()
	}

	if err != nil || (resp.StatusCode >= 400) {
		log.Println(self.Name, " going down. Error: ", err)
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
