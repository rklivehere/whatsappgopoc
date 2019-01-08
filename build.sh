#!/usr/bin/env bash

rm -rf ./whatsapp
CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o whatsapp .