#!/bin/sh
APP=$1
BACKEND=$2
ENDPOINT=$3

redis-cli sadd knuckles:applications ${APP}
redis-cli set knuckles:${APP}:backends:${BACKEND} ${ENDPOINT}
redis-cli publish knuckles:reload 1
