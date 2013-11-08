#!/bin/sh
APP=$1
HOSTNAME=$2

redis-cli sadd knuckles:applications ${APP}
redis-cli sadd knuckles:${APP}:hostnames ${HOSTNAME}
redis-cli publish knuckles:reload 1
