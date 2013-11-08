#!/bin/sh
APP=$1
redis-cli smembers knuckles:${APP}:hostnames
