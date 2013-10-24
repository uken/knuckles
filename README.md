_Guarding our Chaos Emeralds_

## Overview

Provides hostname-based load balancing for HTTP and WebSocket requests.

Configuration is stored in `etcd`, having no downtime during reconfiguration.


# Usage

    ./knuckles -etcd http://localhost:4001/ -etcd_keyspace /knuckles_ssl -listen :443 -proto https

# Etcd Keys
    /knuckles_ssl/<alias>/hostnames/<hostname> = x
    /knuckles_ssl/<alias>/backends/<backend_name> = HTTP endpoint (ex: be01.mycompany.com:80)

Changing keys' TTL does not trigger a config reload.