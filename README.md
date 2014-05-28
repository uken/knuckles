_Guarding our Chaos Emeralds_

[![Build Status](https://travis-ci.org/uken/knuckles.svg?branch=master)](https://travis-ci.org/uken/knuckles)

## Overview

Provides hostname-based load balancing for HTTP and WebSocket requests.

Configuration is stored on `redis`, having no downtime during reconfiguration.


## Usage

    go get -u github.com/uken/knuckles/knuckles
    ./knuckles -config path_to_knuckles_config.conf

Check this [config sample](knuckles/knuckles.sample.conf)

## Operation

Assuming you've setup API on port 8082:

    # Listing applications
    curl localhost:8082/api -d action=list

    # Adding applications
    curl localhost:8082/api -d action=add-application -d application=google

    # Adding hostnames
    curl localhost:8082/api -d action=add-hostname -d application=google -d hostname=xoogle.com

    # Adding backends
    curl localhost:8082/api -d action=add-backend -d application=google -d backend=google.com:80 -d ttl=0

    # Application info
    curl localhost:8082/api -d action=info -d application=google

    # Removing applications
    curl localhost:8082/api -d action=del-application -d application=google

    # Removing hostnames
    curl localhost:8082/api -d action=del-hostname -d application=google -d hostname=xoogle.com

    # Removing backends
    curl localhost:8082/api -d action=del-backend -d application=google -d backend=google.com:80

Please note that `add-backend` takes an extra parameter `ttl`, which dictates for how long the backend should be considered alive. 
This allows services to register and send constant keep-alives.

Sending `ttl=0` disables ttl checking for a specific backend.

## Deployment Considerations

### Security

Make sure the API endpoint is properly secured as it is not authenticated. 
Running on '127.0.0.1:8082' and adding basic auth via reverse proxy is a good option.

### Performance
Redis is the bottleneck. Each HTTP request generates 2 Redis queries:
- Hostname check
- Backend selection

Out of the critical-path, we have the Pinger service which takes care of health-checks. 
You might select a few instances (active proxy or dedicated) for this purpose as the configuration is separated.
Pinger does a lot more Redis queries, but can and should be throttled. By default it scans 1 application 
per second regardless the number of backends.

It's a design trade-off so proxy instances can be totally stateless.

Things go fine with multiple proxies and a single Redis instance until 30k req/s. After that,
the deployment guideline is:

      [clients]      [clients]      [clients]       [clients]
    +------------+ +------------+ +------------+ +------------+
    |  knuckles  | |  knuckles  | |  knuckles  | |  knuckles  |
    |  pinger    | |  pinger    | |  pinger    | |  pinger    |
    | redis-slave| | redis-slave| | redis-slave| | redis-slave|
    +------------+ +------------+ +------------+ +------------+
          |                   |       |                 |
          |                   |       |                 |
          |                +------------+               |
          -----------------|redis-master|----------------
                           |            |
                           |  knuckles  |
                           +------------+
                            [API Access]
