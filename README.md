_Guarding our Chaos Emeralds_

## Overview

Provides hostname-based load balancing for HTTP and WebSocket requests.

Configuration is stored on `redis`, having no downtime during reconfiguration.


## Usage

    go get -u github.com/uken/knuckles/knuckles
    ./knuckles -config path_to_knuckles_config.conf

Check this [config sample](knuckles/knuckles.sample.conf)

## Operation

## Redis

## Deployment Considerations

Redis is the bottleneck. Each HTTP request generates 3 Redis queries:
- Hostname check
- Backend selection
- Backend state

It's a design trade-off so proxy instances can be totally stateless.

Things go fine with multiple proxies and a single Redis instance until 30k req/s. After that,
the deployment guideline is:

      [clients]      [clients]      [clients]       [clients]
    +------------+ +------------+ +------------+ +------------+
    |  knuckles  | |  knuckles  | |  knuckles  | |  knuckles  |
    |            | |            | |            | |            |
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
