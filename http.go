package knuckles

import (
	"fmt"
	etcd "github.com/coreos/go-etcd/etcd"
	"io"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// stats for individual frontends and backends
// flushed at interval N
type HTTPStats struct {
}

type HTTPProxy struct {
	mtx           sync.RWMutex
	etcClient     *etcd.Client
	configVersion uint64
	HostMap       map[string]*Frontend
	Frontends     []*Frontend
	quitChan      chan bool
	Server        http.Server
	Settings      HTTPProxySettings
}

type HTTPProxySettings struct {
	EtcEndpoint            []string
	EtcKeyspace            string
	Endpoint               string
	CheckInterval          time.Duration
	SSL                    bool
	RedirectOnHostnameMiss string
	RedirectOnBackendMiss  string
	RedirectOnError        string
}

func NewHTTPProxy(settings HTTPProxySettings) (*HTTPProxy, error) {
	proxy := new(HTTPProxy)
	proxy.HostMap = make(map[string]*Frontend)
	proxy.Frontends = make([]*Frontend, 0)
	proxy.Settings = settings
	proxy.Server.Addr = settings.Endpoint
	mux := http.NewServeMux()
	mux.Handle("/", proxy)
	proxy.Server.Handler = mux

	proxy.etcClient = etcd.NewClient(settings.EtcEndpoint)
	proxy.etcClient.SyncCluster()
	proxy.quitChan = make(chan bool)
	ok := proxy.Reload()
	if !ok {
		return nil, fmt.Errorf("Failed to load initial configuration from etcd")
	}
	return proxy, nil
}

func (self *HTTPProxy) Start() {
	run := true

	ch := make(chan *etcd.Response)
	stop := make(chan bool)

	go self.Server.ListenAndServe()

	go self.etcClient.Watch(self.Settings.EtcKeyspace, self.configVersion+1, ch, stop)

	for run {
		select {
		case <-self.quitChan:
			stop <- true
			run = false
		case resp := <-ch:
			// don't reload in case of TTL updates
			if resp.PrevValue != resp.Value {
				self.Reload()
			}
		}
	}
	self.quitChan <- true
}

func (self *HTTPProxy) Stop() {
	self.quitChan <- true
	<-self.quitChan
	for _, f := range self.HostMap {
		f.Stop()
	}
	close(self.quitChan)
}

func (self *HTTPProxy) etcGet(key string) ([]*etcd.Response, error) {
	return self.etcClient.Get(fmt.Sprintf("%s/%s", self.Settings.EtcKeyspace, key))
}

func (self *HTTPProxy) loadHosts(appKey string, frontend *Frontend, newHostMap map[string]*Frontend) error {
	hostList, err := self.etcGet(fmt.Sprintf("%s/hostnames", appKey))
	if err != nil || len(hostList) < 1 {
		return fmt.Errorf("No hostnames")
	}

	for _, hostEntry := range hostList {
		hostKey := lastSep(hostEntry.Key)
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
	backendList, err := self.etcGet(fmt.Sprintf("%s/backends", appKey))
	if err != nil || len(backendList) < 1 {
		return fmt.Errorf("No backends")
	}

	for _, backendEntry := range backendList {
		beKey := lastSep(backendEntry.Key)
		endpoint := backendEntry.Value

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
	// traverse etcd structure
	// lock/unlock proxy for least amount of time
	log.Println("Config loading")
	var configIndex uint64
	appList, err := self.etcGet("")
	if err != nil {
		log.Println("Could not load application list")
		return false
	}

	newHostMap := make(map[string]*Frontend)
	newFrontends := make([]*Frontend, 0)

	for _, app := range appList {
		configIndex = app.Index
		appKey := lastSep(app.Key)
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
	//oldHostMap := self.HostMap
	self.HostMap = newHostMap
	self.configVersion = configIndex
	self.mtx.Unlock()

	log.Println("Stopping old frontends")
	for _, frontend := range oldFrontends {
		log.Println("Stopping old frontend ", frontend.Name)
		frontend.Stop()
	}

	log.Println("Load Complete")

	return true
}

func (self *HTTPProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	self.mtx.RLock()
	defer self.mtx.RUnlock()

	hostname := r.Host

	if sep := strings.Index(hostname, ":"); sep >= 0 {
		hostname = hostname[:sep]
	}

	frontend, ok := self.HostMap[hostname]
	if !ok {
		http.Redirect(w, r, self.Settings.RedirectOnHostnameMiss, http.StatusTemporaryRedirect)
		return
	}

	backend, err := frontend.PickBackend()

	if err != nil {
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
		self.simpleProxy(w, r)
	}

}

func (self *HTTPProxy) simpleProxy(w http.ResponseWriter, r *http.Request) {
	resp, err := http.DefaultTransport.RoundTrip(r)

	if err != nil {
		http.Redirect(w, r, self.Settings.RedirectOnError, http.StatusTemporaryRedirect)
		log.Println(err)
		return
	}
	defer resp.Body.Close()

	for k, v := range resp.Header {
		for _, vv := range v {
			w.Header().Add(k, vv)
		}
	}

	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

func (self *HTTPProxy) upgradeConnection(w http.ResponseWriter, r *http.Request) {
	hj, ok := w.(http.Hijacker)

	if !ok {
		http.Redirect(w, r, self.Settings.RedirectOnError, http.StatusTemporaryRedirect)
		return
	}

	client, _, err := hj.Hijack()
	if err != nil {
		http.Redirect(w, r, self.Settings.RedirectOnError, http.StatusTemporaryRedirect)
		client.Close()
		return
	}

	server, err := net.Dial("tcp", r.URL.Host)
	if err != nil {
		http.Redirect(w, r, self.Settings.RedirectOnError, http.StatusTemporaryRedirect)
		client.Close()
		return
	}

	err = r.Write(server)
	if err != nil {
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
	parts := strings.Split(k, "/")
	return parts[len(parts)-1]
}

func requestStart() string {
	return strconv.FormatInt(time.Now().UnixNano()/int64(time.Millisecond), 10)
}
