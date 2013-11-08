_Guarding our Chaos Emeralds_

## Overview

Provides hostname-based load balancing for HTTP and WebSocket requests.

Configuration is stored in `redis`, having no downtime during reconfiguration.


# Usage

    go get -u github.com/uken/knuckles/knuckles
    ./knuckles -config path_to_config.yml

Check this [config sample](knuckles/config.sample.yml)

# Redis Keys
    knuckles:applications = SET with applications
    knuckles:reload = PubSub for reload operation
    knuckles:<application>:hostnames = SET with 'Host' fields
    knuckles:<application>:backends:<backend_name> = HTTP endpoint (ex: be01.mycompany.com:80)
    # check script directory for examples

Changing keys' TTL does not trigger a config reload.
