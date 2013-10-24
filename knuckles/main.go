package main

import (
	"github.com/uken/knuckles"
	"log"
)

func main() {
	settings := knuckles.HTTPProxySettings{
		EtcKeyspace: "/knuckles",
		Endpoint:    ":8080",
	}
	proxy, err := knuckles.NewHTTPProxy(settings)

	if err != nil {
		log.Println(err)
		return
	}
	proxy.Start()
}
