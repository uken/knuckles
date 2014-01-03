package knuckles

import (
	"fmt"
	"github.com/fiorix/go-redis/redis"
	"log"
	"strings"
	"time"
)

type redisDriver struct {
	redisClient *redis.Client
	namespace   string
	quitChan    chan bool
	running     bool
}

func (self *redisDriver) Load(checkInterval time.Duration) (*HTTPConfig, error) {
	// not optimal, but still decent
	// traverse disc structure
	// lock/unlock proxy for least amount of time
	var err error
	var appList []string

	for retries := 0; retries < 2; retries++ {
		appList, err = self.list("applications")
		if err != nil {
			log.Println("Failed to load application list, retry count", retries)
		} else {
			break
		}
	}

	if err != nil {
		log.Println("Could not load application list:", err)
		return nil, err
	}

	config := NewHTTPConfig()

	for _, app := range appList {
		appKey := self.lastSep(app)
		log.Println("Loading app ", appKey)

		frontend := NewFrontend(appKey, checkInterval)

		err = self.loadHosts(appKey, frontend, config.HostMap)
		if err != nil {
			log.Println("Error loading hosts:", err)
			continue
		}

		err = self.loadBackends(appKey, frontend, checkInterval)
		if err != nil {
			log.Println("Error loading backends:", err)
			continue
		}

		config.Frontends = append(config.Frontends, frontend)
	}

	return config, nil
}

func (self *redisDriver) Start() chan bool {
	running := true
	retChan := make(chan bool)

	go func() {
		ch := make(chan redis.PubSubMessage)
		stop := make(chan bool)
		channel := fmt.Sprintf("%s:reload", self.namespace)
		for running {
			err := self.redisClient.Subscribe(channel, ch, stop)

			if err != nil {
				log.Println("redis error ", err)
				time.Sleep(1 * time.Second)
				continue
			}

			for running {
				select {
				case <-self.quitChan:
					running = false
				case <-time.After(30 * time.Second):
					self.ping()
				case resp := <-ch:
					if resp.Error != nil {
						log.Println(resp.Error)
						log.Println("redis notification error ", err)
						break
					} else {
						retChan <- true
					}
				}
			}
		}
	}()
	return retChan
}

func (self *redisDriver) ping() {
	self.redisClient.Ping()
}

func (self *redisDriver) Stop() {
	self.quitChan <- true
}

func (self *redisDriver) Config(hosts []string, namespace string) error {
	self.redisClient = redis.New(hosts...)
	self.namespace = namespace
	self.quitChan = make(chan bool)
	return nil
}

func (self *redisDriver) keySpaced(key string) string {
	return fmt.Sprintf("%s:%s", self.namespace, key)
}

func (self *redisDriver) get(key string) (string, error) {
	return self.redisClient.Get(self.keySpaced(key))
}

func (self *redisDriver) list(key string) ([]string, error) {
	return self.redisClient.SMembers(self.keySpaced(key))
}

func (self *redisDriver) find(key string) ([]string, error) {
	return self.redisClient.Keys(fmt.Sprintf("%s:*", self.keySpaced(key)))
}

func (self *redisDriver) loadHosts(appKey string, frontend *Frontend, newHostMap map[string]*Frontend) error {
	hostList, err := self.list(fmt.Sprintf("%s:hostnames", appKey))
	if err != nil || len(hostList) < 1 {
		return fmt.Errorf("No hostnames")
	}

	for _, hostEntry := range hostList {
		hostKey := self.lastSep(hostEntry)
		_, present := newHostMap[hostKey]
		if present {
			log.Println("Ignoring duplicated hostname ", hostKey)
			continue
		}
		newHostMap[hostKey] = frontend
	}
	return nil
}

func (self *redisDriver) loadBackends(appKey string, frontend *Frontend, checkInterval time.Duration) error {
	backendList, err := self.find(fmt.Sprintf("%s:backends", appKey))
	if err != nil || len(backendList) < 1 {
		return fmt.Errorf("No backends")
	}

	for _, backendEntry := range backendList {
		beKey := self.lastSep(backendEntry)
		endpoint, err := self.get(fmt.Sprintf("%s:backends:%s", appKey, beKey))
		if err != nil {
			log.Println("Skipping invalid backend ", beKey)
			continue
		}
		frontend.AddBackend(beKey, endpoint)
	}
	return nil
}

func (self *redisDriver) lastSep(k string) string {
	parts := strings.Split(k, ":")
	return parts[len(parts)-1]
}
