#!/usr/bin/env bash

CGO_ENABLED=0 GOOS=$(uname -s | tr '[:upper:]' '[:lower:]') go build -a -installsuffix gotpl -o gotpl .