package knuckles

import (
	"fmt"
	"github.com/coreos/go-etcd/etcd"
	"log"
	"strings"
	"time"
)

type etcdDriver struct {
	etcdClient  *etcd.Client
	namespace   string
	quitChan    chan bool
	running     bool
	watchIndex  uint64
	loadedIndex uint64
}

func (self *etcdDriver) Load(checkInterval time.Duration) (*HTTPConfig, error) {
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

	self.loadedIndex = self.index("")

	return config, nil
}

func (self *etcdDriver) Start() chan bool {
	self.running = true

	retChan := make(chan bool)
	stop := make(chan bool)
	ch := make(chan *etcd.Response, 10)

	// self.etcdClient.OpenCURL()
	// go func() {
	//	for self.running {
	//		r := self.etcdClient.RecvCURL()
	//		log.Println(r)
	//	}
	// }()

	go func() {
		for self.running {
			self.watchIndex = self.loadedIndex
			log.Println("etcD watching global index", self.watchIndex)
			_, err := self.etcdClient.Watch(self.namespace, self.watchIndex+1, true, ch, stop)
			if err != nil {
				log.Println("etcd error ", err)
				time.Sleep(1 * time.Second)
			}
		}
	}()

	go func() {
		for self.running {
			select {
			case <-self.quitChan:
				break
			case <-time.After(30 * time.Second):
				self.ping()
			case resp := <-ch:
				if resp.PrevNode == nil ||
					resp.Action == "expire" ||
					(resp.PrevNode != nil && resp.Node != nil && resp.PrevNode.Value != resp.Node.Value) {
					retChan <- true
				}
			}
		}
	}()
	return retChan
}

func (self *etcdDriver) ping() {
	self.etcdClient.SyncCluster()
}

func (self *etcdDriver) Stop() {
	self.running = false
	self.quitChan <- true
}

func (self *etcdDriver) Config(hosts []string, namespace string) error {
	self.etcdClient = etcd.NewClient(hosts)
	self.namespace = namespace
	self.quitChan = make(chan bool)
	return nil
}

func (self *etcdDriver) keySpaced(key string) string {
	return fmt.Sprintf("%s/%s", self.namespace, key)
}

func (self *etcdDriver) index(key string) uint64 {
	resp, err := self.etcdClient.Get(self.keySpaced(key), false, false)
	if err != nil {
		return 0
	} else {
		return resp.EtcdIndex
	}
}

func (self *etcdDriver) get(key string) (string, error) {
	resp, err := self.etcdClient.Get(self.keySpaced(key), false, false)
	if err != nil {
		return "", err
	} else {
		return resp.Node.Value, nil
	}
}

func (self *etcdDriver) list(key string) ([]string, error) {
	var ret []string

	resp, err := self.etcdClient.Get(self.keySpaced(key), false, false)

	if err != nil {
		return ret, err
	}

	for _, node := range resp.Node.Nodes {
		ret = append(ret, self.lastSep(node.Key))
	}

	return ret, nil
}

func (self *etcdDriver) loadHosts(appKey string, frontend *Frontend, newHostMap map[string]*Frontend) error {
	hostList, err := self.list(fmt.Sprintf("applications/%s/hostnames", appKey))
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

func (self *etcdDriver) loadBackends(appKey string, frontend *Frontend, checkInterval time.Duration) error {
	backendList, err := self.list(fmt.Sprintf("applications/%s/backends", appKey))
	if err != nil || len(backendList) < 1 {
		return fmt.Errorf("No backends")
	}

	for _, backendEntry := range backendList {
		beKey := self.lastSep(backendEntry)
		endpoint, err := self.get(fmt.Sprintf("applications/%s/backends/%s", appKey, beKey))
		if err != nil {
			log.Println("Skipping invalid backend ", beKey)
			continue
		}
		frontend.AddBackend(beKey, endpoint)
	}
	return nil
}

func (self *etcdDriver) lastSep(k string) string {
	parts := strings.Split(k, "/")
	return parts[len(parts)-1]
}
