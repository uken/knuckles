package main

import (
  "flag"
  "github.com/BurntSushi/toml"
  "github.com/uken/knuckles"
  "log"
  "os"
  "os/signal"
  "sync"
  "syscall"
)

type apiFormat struct {
  Address string
}

type redisFormat struct {
  Address   string
  Namespace string
}

type pingerFormat struct {
  Redis     string
  Namespace string
  Interval  int
}

type listenerFormat struct {
  Address         string
  XRequestStart   bool   `toml:"x_request_start"`
  XForwardedFor   bool   `toml:"x_forwarded_for"`
  XForwardedProto string `toml:"x_forwarded_proto"`
  ErrorNoBackend  string `toml:"error_no_backend"`
  ErrorNoHostname string `toml:"error_no_hostname"`
  ErrorInternal   string `toml:"error_internal"`
}

type configFormat struct {
  Api       apiFormat
  Listeners map[string]listenerFormat
  Redis     redisFormat
  Pinger    pingerFormat
}

var configFile = flag.String("config", "/etc/knuckles.conf", "Configuration File")
var proxy *knuckles.HTTPProxy

func main() {
  var err error
  var wg sync.WaitGroup
  var proxies []*knuckles.HTTPProxy
  var pinger *knuckles.Pinger

  flag.Parse()

  if *configFile == "" {
    flag.PrintDefaults()
    os.Exit(2)
  }

  var config configFormat

  _, err = toml.DecodeFile(*configFile, &config)

  if err != nil {
    log.Println(err)
    os.Exit(1)
  }

  store, err := knuckles.NewRedisStore(config.Redis.Namespace, config.Redis.Address)

  if err != nil {
    log.Println(err)
    os.Exit(1)
  }

  apiConfig := knuckles.HTTPAPIConfig{}
  apiConfig.Addr = config.Api.Address
  apiConfig.Store = store

  api, err := knuckles.NewHTTPAPI(apiConfig)

  if err != nil {
    log.Println(err)
    os.Exit(1)
  }

  if config.Pinger.Redis != "" {
    log.Println("Starting PING service")
    pinger, err = knuckles.NewPinger(config.Pinger.Namespace, config.Pinger.Redis, config.Pinger.Interval)
    if err != nil {
      log.Println(err)
      os.Exit(1)
    }
    wg.Add(1)
  }

  for lName, lF := range config.Listeners {

    lConf := knuckles.HTTPProxyConfig{
      Store:                 store,
      Addr:                  lF.Address,
      XForwardedFor:         lF.XForwardedFor,
      XForwardedProto:       lF.XForwardedProto,
      XRequestStart:         lF.XRequestStart,
      RedirectNoHostname:    lF.ErrorNoHostname,
      RedirectNoBackend:     lF.ErrorNoBackend,
      RedirectInternalError: lF.ErrorInternal,
    }

    listener, err := knuckles.NewHTTPProxy(lConf)

    if err != nil {
      log.Println(err)
      os.Exit(1)
    }

    log.Println("Adding listener", lName, lF.Address)

    proxies = append(proxies, listener)
  }

  // terminate on ctrl+c or via kill
  signalC := make(chan os.Signal)
  signal.Notify(signalC, os.Interrupt, syscall.SIGTERM)
  go func() {
    <-signalC
    for _, proxy := range proxies {
      proxy.Stop()
    }
    api.Stop()
    wg.Done()

    if config.Pinger.Redis != "" {
      pinger.Stop()
      wg.Done()
    }

  }()

  for _, proxy := range proxies {
    go func(p *knuckles.HTTPProxy) {
      wg.Add(1)
      p.Start()
      wg.Done()
    }(proxy)
  }

  wg.Add(1)
  go api.Start()

  if config.Pinger.Redis != "" {
    go pinger.Start()
  }

  wg.Wait()
  log.Println("Stopped")
}
