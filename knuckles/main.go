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

	if settings.EtcKeyspace, err = config.Get("etcd.keyspace"); err != nil {
		log.Println(err)
		os.Exit(1)
	}

	if checkInterval, err := config.GetInt("http.check_interval"); err != nil {
		log.Println(err)
		os.Exit(1)
	} else {
		settings.CheckInterval = time.Duration(checkInterval) * time.Millisecond
	}

	if stunnel, err := config.GetBool("http.stunnel"); err != nil {
		log.Println(err)
		os.Exit(1)
	} else {
		if stunnel {
			settings.SSL = true
		} else {
			settings.SSL = false
		}
	}

	etcdHostCount, err := config.Count("etcd.hosts")
	if err != nil || etcdHostCount < 1 {
		log.Println("Missing etcd hosts")
		os.Exit(1)
	}

	log.Println("Adding ", etcdHostCount, " etcd hosts")
	settings.EtcEndpoint = []string{}
	for i := 0; i < etcdHostCount; i++ {
		k := fmt.Sprintf("etcd.hosts[%d]", i)
		if addr, err := config.Get(k); err != nil {
			log.Println(err)
			os.Exit(1)
			return
		} else {
			settings.EtcEndpoint = append(settings.EtcEndpoint, addr)
		}
	}

	log.Println(settings.EtcEndpoint)
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
