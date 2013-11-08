package knuckles

import (
	"fmt"
	"github.com/fiorix/go-redis/redis"
	"io"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

type HTTPProxy struct {
	mtx         sync.RWMutex
	redisClient *redis.Client
	HostMap     map[string]*Frontend
	Frontends   []*Frontend
	quitChan    chan bool
	Server      http.Server
	Settings    HTTPProxySettings
	HttpStatus  http.Server
	status      *HTTPStats
	running     bool
}

type HTTPProxySettings struct {
	DiscEndpoint           []string
	DiscKeyspace           string
	Endpoint               string
	StatusEndpoint         string
	StatusPrefix           string
	CheckInterval          time.Duration
	SSL                    bool
	RedirectOnHostnameMiss string
	RedirectOnBackendMiss  string
	RedirectOnError        string
}

func NewHTTPProxy(settings HTTPProxySettings) (*HTTPProxy, error) {
	var err error

	proxy := new(HTTPProxy)
	proxy.HostMap = make(map[string]*Frontend)
	proxy.Frontends = make([]*Frontend, 0)
	proxy.Settings = settings
	proxy.Server.Addr = settings.Endpoint
	mux := http.NewServeMux()
	mux.Handle("/", proxy)
	proxy.Server.Handler = mux

	proxy.status, err = NewHTTPStats(settings.StatusEndpoint, settings.StatusPrefix)
	if err != nil {
		return nil, err
	}

	proxy.redisClient = redis.New(settings.DiscEndpoint...)

	proxy.quitChan = make(chan bool)

	ok := proxy.Reload()
	if !ok {
		return nil, fmt.Errorf("Failed to load initial configuration from discovery engine")
	}
	return proxy, nil
}

func (self *HTTPProxy) Start() {
	self.running = true

	ch := make(chan redis.PubSubMessage)
	stop := make(chan bool)

	go self.Server.ListenAndServe()
	go self.status.Start()

	go self.Watch(ch, stop)

	for self.running {
		select {
		case <-self.quitChan:
			self.running = false
			stop <- true
		case resp := <-ch:
			// only reload on legitimate notifications
			if resp.Error == nil {
				self.Reload()
			}
		}
	}
	self.quitChan <- true
}

// also step locked to main thread
func (self *HTTPProxy) Watch(callerCh chan redis.PubSubMessage, stop chan bool) {
	ch := make(chan redis.PubSubMessage)
	channel := fmt.Sprintf("%s:reload", self.Settings.DiscKeyspace)

	for self.running {
		err := self.redisClient.Subscribe(channel, ch, stop)

		if err != nil {
			log.Println("Discovery error ", err)
			time.Sleep(1 * time.Second)
			continue
		}

		for {
			resp := <-ch
			if resp.Error != nil {
				log.Println(resp.Error)
				log.Println("Discovery notification error ", err)
				break
			} else {
				callerCh <- resp
			}
		}

	}
}

func (self *HTTPProxy) Stop() {
	self.quitChan <- true
	<-self.quitChan
	for _, f := range self.HostMap {
		f.Stop()
	}
	self.status.Stop()
	close(self.quitChan)
}

func (self *HTTPProxy) keySpaced(key string) string {
	return fmt.Sprintf("%s:%s", self.Settings.DiscKeyspace, key)
}

func (self *HTTPProxy) discGet(key string) (string, error) {
	return self.redisClient.Get(self.keySpaced(key))
}

func (self *HTTPProxy) discList(key string) ([]string, error) {
	return self.redisClient.SMembers(self.keySpaced(key))
}

func (self *HTTPProxy) discFind(key string) ([]string, error) {
	return self.redisClient.Keys(fmt.Sprintf("%s:*", self.keySpaced(key)))
}

func (self *HTTPProxy) loadHosts(appKey string, frontend *Frontend, newHostMap map[string]*Frontend) error {
	hostList, err := self.discList(fmt.Sprintf("%s:hostnames", appKey))
	if err != nil || len(hostList) < 1 {
		return fmt.Errorf("No hostnames")
	}

	for _, hostEntry := range hostList {
		hostKey := lastSep(hostEntry)
		_, present := newHostMap[hostKey]
		if present {
			log.Println("Ignoring duplicated hostname ", hostKey)
			continue
		}
		newHostMap[hostKey] = frontend
	}
	return nil
}

func (self *HTTPProxy) loadBackends(appKey string, frontend *Frontend) error {
	backendList, err := self.discFind(fmt.Sprintf("%s:backends", appKey))
	if err != nil || len(backendList) < 1 {
		return fmt.Errorf("No backends")
	}

	for _, backendEntry := range backendList {
		beKey := lastSep(backendEntry)
		endpoint, err := self.discGet(fmt.Sprintf("%s:backends:%s", appKey, beKey))
		if err != nil {
			log.Println("Skipping invalid backend ", beKey)
			continue
		}

		settings := BackendSettings{
			Endpoint:      endpoint,
			CheckInterval: self.Settings.CheckInterval,
			Updates:       frontend.NotifyChan,
			CheckUrl:      fmt.Sprintf("http://%s", endpoint),
		}
		backend, err := NewBackend(beKey, settings)
		if err != nil {
			log.Println("Skipping invalid backend ", beKey)
			continue
		}
		frontend.AddBackend(backend)
	}
	return nil
}

func (self *HTTPProxy) Reload() bool {
	// not optimal, but still decent
	// traverse disc structure
	// lock/unlock proxy for least amount of time
	log.Println("Config loading")
	appList, err := self.discList("applications")
	if err != nil {
		log.Println("Could not load application list")
		return false
	}

	newHostMap := make(map[string]*Frontend)
	newFrontends := make([]*Frontend, 0)

	for _, app := range appList {
		appKey := lastSep(app)
		log.Println("Loading app ", appKey)

		frontend := NewFrontend(appKey)

		err = self.loadHosts(appKey, frontend, newHostMap)
		if err != nil {
			log.Println(err)
			continue
		}

		err = self.loadBackends(appKey, frontend)
		if err != nil {
			log.Println(err)
			continue
		}

		newFrontends = append(newFrontends, frontend)
	}

	for _, fr := range newFrontends {
		log.Println("Starting new frontend ", fr.Name)
		go fr.Start()
	}

	log.Println("Replacing configuration")
	self.mtx.Lock()
	oldFrontends := self.Frontends
	self.Frontends = newFrontends
	self.HostMap = newHostMap
	self.mtx.Unlock()

	log.Println("Stopping old frontends")
	for _, frontend := range oldFrontends {
		log.Println("Stopping old frontend ", frontend.Name)
		frontend.Stop()
	}

	log.Println("Load Complete")
	self.status.Increment(MetricReload)

	return true
}

func (self *HTTPProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	self.mtx.RLock()
	defer self.mtx.RUnlock()

	self.status.Increment(MetricRequest)

	hostname := r.Host

	if sep := strings.Index(hostname, ":"); sep >= 0 {
		hostname = hostname[:sep]
	}

	frontend, ok := self.HostMap[hostname]
	if !ok {
		self.status.Increment(MetricNoHostname)
		http.Redirect(w, r, self.Settings.RedirectOnHostnameMiss, http.StatusTemporaryRedirect)
		return
	}

	backend, err := frontend.PickBackend()

	if err != nil {
		self.status.Increment(MetricNoBackend)
		http.Redirect(w, r, self.Settings.RedirectOnBackendMiss, http.StatusTemporaryRedirect)
		return
	}

	r.Header.Set("X-Request-Start", requestStart())

	proto := "http"
	if self.Settings.SSL {
		proto = "https"
		//stunnel already adds X-Forwarded-For
	} else {
		r.Header.Set("X-Forwarded-For", r.RemoteAddr)
	}

	r.Header.Set("X-Forwarded-Proto", proto)

	r.URL.Host = backend.Endpoint
	r.URL.Scheme = "http"

	connection := r.Header.Get("Connection")
	if strings.ToLower(connection) == "upgrade" {
		self.upgradeConnection(w, r)
	} else {
		respStatus := self.simpleProxy(w, r)
		self.status.IncrementBackend(frontend.Name, backend.Name, respStatus)
	}

}

func (self *HTTPProxy) simpleProxy(w http.ResponseWriter, r *http.Request) int {
	resp, err := http.DefaultTransport.RoundTrip(r)

	if err != nil {
		self.status.Increment(MetricError)
		http.Redirect(w, r, self.Settings.RedirectOnError, http.StatusTemporaryRedirect)
		log.Println(err)
		return http.StatusTemporaryRedirect
	}
	defer resp.Body.Close()

	for k, v := range resp.Header {
		for _, vv := range v {
			w.Header().Add(k, vv)
		}
	}

	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
	return resp.StatusCode
}

func (self *HTTPProxy) upgradeConnection(w http.ResponseWriter, r *http.Request) {
	hj, ok := w.(http.Hijacker)

	if !ok {
		self.status.Increment(MetricError)
		http.Redirect(w, r, self.Settings.RedirectOnError, http.StatusTemporaryRedirect)
		return
	}

	client, _, err := hj.Hijack()
	if err != nil {
		self.status.Increment(MetricError)
		http.Redirect(w, r, self.Settings.RedirectOnError, http.StatusTemporaryRedirect)
		client.Close()
		return
	}

	server, err := net.Dial("tcp", r.URL.Host)
	if err != nil {
		self.status.Increment(MetricError)
		http.Redirect(w, r, self.Settings.RedirectOnError, http.StatusTemporaryRedirect)
		client.Close()
		return
	}

	err = r.Write(server)
	if err != nil {
		self.status.Increment(MetricError)
		log.Printf("writing WebSocket request to backend server failed: %v", err)
		server.Close()
		client.Close()
		return
	}

	// we spawn a goroutine so the caller can unlock (and reload)
	go passBytes(client, server)
}

func passBytes(client, server net.Conn) {
	pass := func(from, to net.Conn, done chan error) {
		//TODO check frame sizes
		//TODO half writes
		_, err := io.Copy(to, from)
		done <- err
	}

	done := make(chan error, 2)
	go pass(client, server, done)
	go pass(server, client, done)

	<-done
	//first to error kills both sides
	client.Close()
	server.Close()
	//wait for second side to exit
	<-done
	close(done)
}

func lastSep(k string) string {
	parts := strings.Split(k, ":")
	return parts[len(parts)-1]
}

func requestStart() string {
	return strconv.FormatInt(time.Now().UnixNano()/int64(time.Millisecond), 10)
}
