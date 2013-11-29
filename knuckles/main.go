package main

import (
	"flag"
	"fmt"
	"github.com/kylelemons/go-gypsy/yaml"
	"github.com/uken/knuckles"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
)

var configFile = flag.String("config", "", "Configuration File")
var proxy *knuckles.HTTPProxy

func main() {
	flag.Parse()

	if *configFile == "" {
		flag.PrintDefaults()
		os.Exit(2)
	}

	config, err := yaml.ReadFile(*configFile)
	if err != nil {
		log.Println(err)
		os.Exit(1)

	}

	var settings knuckles.HTTPProxySettings
	if settings.Endpoint, err = config.Get("http.address"); err != nil {
		log.Println(err)
		os.Exit(1)
	}

	if settings.StatusEndpoint, err = config.Get("statsd.address"); err != nil {
		log.Println(err)
		os.Exit(1)
	}

	if settings.StatusPrefix, err = config.Get("statsd.prefix"); err != nil {
		log.Println(err)
		os.Exit(1)
	}

	if settings.RedirectOnError, err = config.Get("http.redirect.error"); err != nil {
		log.Println(err)
		os.Exit(1)
	}

	if settings.RedirectOnBackendMiss, err = config.Get("http.redirect.no_backend"); err != nil {
		log.Println(err)
		os.Exit(1)
	}

	if settings.RedirectOnHostnameMiss, err = config.Get("http.redirect.no_hostname"); err != nil {
		log.Println(err)
		os.Exit(1)
	}

	if settings.DiscKeyspace, err = config.Get("redis.keyspace"); err != nil {
		log.Println(err)
		os.Exit(1)
	}

	if checkInterval, err := config.GetInt("http.check_interval"); err != nil {
		log.Println(err)
		os.Exit(1)
	} else {
		settings.CheckInterval = time.Duration(checkInterval) * time.Millisecond
	}

	if stunnel, err := config.GetBool("http.x_forwarded_for"); err != nil {
		log.Println(err)
		os.Exit(1)
	} else {
		settings.XForwardedFor = stunnel
	}

	if req_start, err := config.GetBool("http.x_request_start"); err != nil {
		log.Println(err)
		os.Exit(1)
	} else {
		settings.XRequestStart = req_start
	}

	if settings.XForwardedProto, err = config.Get("http.x_forwarded_proto"); err != nil {
		log.Println(err)
		os.Exit(1)
	}
	discHostCount, err := config.Count("redis.hosts")
	if err != nil || discHostCount < 1 {
		log.Println("Missing redis hosts")
		os.Exit(1)
	}

	log.Println("Adding ", discHostCount, " redis hosts")
	settings.DiscEndpoint = []string{}
	for i := 0; i < discHostCount; i++ {
		k := fmt.Sprintf("redis.hosts[%d]", i)
		if addr, err := config.Get(k); err != nil {
			log.Println(err)
			os.Exit(1)
			return
		} else {
			settings.DiscEndpoint = append(settings.DiscEndpoint, addr)
		}
	}

	log.Println(settings.DiscEndpoint)
	proxy, err = knuckles.NewHTTPProxy(settings)

	if err != nil {
		log.Println(err)
		os.Exit(1)
	}

	// terminate on ctrl+c or via kill
	signalC := make(chan os.Signal, 1)
	signal.Notify(signalC, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-signalC
		proxy.Stop()
	}()

	proxy.Start()
	log.Println("Stopped")
}
