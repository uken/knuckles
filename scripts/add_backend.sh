#!/bin/sh
APP=$1
BACKEND=$2
ENDPOINT=$3
curl -L http://127.0.0.1:4001/v1/keys/knuckles/${APP}/backends/${BACKEND} -d value=${ENDPOINT}
