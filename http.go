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
	config      *HTTPConfig
	quitChan    chan bool
	Server      http.Server
	Settings    HTTPProxySettings
	HttpStatus  http.Server
	status      *HTTPStats
	running     bool
	discovery   DiscoveryListener
}

type HTTPProxySettings struct {
	DiscEndpoint           []string
	DiscKeyspace           string
	Endpoint               string
	StatusEndpoint         string
	StatusPrefix           string
	CheckInterval          time.Duration
	RedirectOnHostnameMiss string
	RedirectOnBackendMiss  string
	RedirectOnError        string
	XForwardedProto        string
	XForwardedFor          bool
	XRequestStart          bool
}

func NewHTTPProxy(settings HTTPProxySettings) (*HTTPProxy, error) {
	var err error

	proxy := new(HTTPProxy)
	proxy.config = NewHTTPConfig()
	proxy.Settings = settings
	proxy.Server.Addr = settings.Endpoint
	mux := http.NewServeMux()
	mux.Handle("/", proxy)
	proxy.Server.Handler = mux

	proxy.status, err = NewHTTPStats(settings.StatusEndpoint, settings.StatusPrefix)
	if err != nil {
		return nil, err
	}

	proxy.discovery = &redisDriver{}
	proxy.discovery.Config(settings.DiscEndpoint, settings.DiscKeyspace)

	proxy.quitChan = make(chan bool)

	ok := proxy.Reload()
	if !ok {
		return nil, fmt.Errorf("Failed to load initial configuration from discovery engine")
	}
	return proxy, nil
}

func (self *HTTPProxy) Start() {
	self.running = true

	go self.Server.ListenAndServe()
	go self.status.Start()

	ch := self.discovery.Start()

	for self.running {
		select {
		case <-self.quitChan:
			self.running = false
		case <-ch:
			self.Reload()
		}
	}
	self.quitChan <- true
}

func (self *HTTPProxy) Stop() {
	self.quitChan <- true
	<-self.quitChan
	for _, f := range self.config.HostMap {
		f.Stop()
	}
	self.status.Stop()
	close(self.quitChan)
}

func (self *HTTPProxy) Reload() bool {
	// not optimal, but still decent
	// traverse disc structure
	// lock/unlock proxy for least amount of time
	log.Println("Config loading")
	newConfig, err := self.discovery.Load(self.Settings.CheckInterval)
	if err != nil {
		log.Println("Config load failed, keeping old config")
		return false
	}

	for _, fr := range newConfig.Frontends {
		log.Println("Starting new frontend ", fr.Name)
		go fr.Start()
	}

	log.Println("Replacing configuration")
	self.mtx.Lock()
	oldConfig := self.config
	self.config = newConfig
	self.mtx.Unlock()

	log.Println("Stopping old frontends")
	for _, frontend := range oldConfig.Frontends {
		log.Println("Stopping old frontend ", frontend.Name)
		frontend.Stop()
	}

	log.Println("Load Complete")
	self.status.Increment(MetricReload)

	return true
}

func (self *HTTPProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	self.mtx.RLock()
	localConfig := self.config
	self.mtx.RUnlock()

	self.status.Increment(MetricRequest)

	hostname := r.Host

	if sep := strings.Index(hostname, ":"); sep >= 0 {
		hostname = hostname[:sep]
	}

	frontend, ok := localConfig.HostMap[hostname]
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

	if self.Settings.XRequestStart {
		r.Header.Set("X-Request-Start", requestStart())
	}

	if self.Settings.XForwardedProto != "" {
		r.Header.Set("X-Forwarded-Proto", self.Settings.XForwardedProto)
	}

	if self.Settings.XForwardedFor {
		r.Header.Set("X-Forwarded-For", r.RemoteAddr)
	}

	r.URL.Host = backend
	r.URL.Scheme = "http"

	connection := r.Header.Get("Connection")
	if strings.ToLower(connection) == "upgrade" {
		self.upgradeConnection(w, r)
	} else {
		respStatus := self.simpleProxy(w, r)
		self.status.IncrementFrontend(frontend.Name, respStatus)
	}

}

func (self *HTTPProxy) simpleProxy(w http.ResponseWriter, r *http.Request) int {
	tr := &http.Transport{
		DisableKeepAlives: true,
	}
	resp, err := tr.RoundTrip(r)

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
