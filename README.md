_Guarding our Chaos Emeralds_

## Overview

Provides hostname-based load balancing for HTTP and WebSocket requests.

Configuration is stored in either `redis` or `etcD`, having no downtime during reconfiguration.


# Usage

    go get -u github.com/uken/knuckles/knuckles
    ./knuckles -config path_to_config.yml

Check this [config sample](knuckles/config.sample.yml)

# Redis

    knuckles:applications = SET with applications
    knuckles:<application>:hostnames = SET with 'Host' fields
    knuckles:<application>:backends:<backend_name> = HTTP endpoint (ex: be01.mycompany.com:80)
    knuckles:reload = PubSub for reload operation
    

Redis requires a reload notification (pubsub above). Publishing any value will reload the config, ex:

    PUBLISH knuckles:reload 1


# etcD

    knuckles/applications/<application>/hostnames/<hostname>
    knuckles/applications/<application>/backends/<backend_name> = HTTP endpoint (ex: be01.mycompany.com:80)


Either way, changing keys' TTL does not trigger a config reload.

Check script directory for examples.