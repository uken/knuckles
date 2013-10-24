#!/bin/sh
APP=$1
HOSTNAME=$2
curl -L http://127.0.0.1:4001/v1/keys/knuckles/${APP}/hostnames/${HOSTNAME} -d value=x
