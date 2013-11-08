#!/bin/sh
APP=$1
redis-cli keys knuckles:${APP}:backends:*
