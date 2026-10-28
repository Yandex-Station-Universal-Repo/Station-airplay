#!/bin/sh
set -eu
cd /repo/source
GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=0 \
  go build -buildvcs=false -trimpath -ldflags='-s -w' -o ../bin/station-airplay .
ls -l ../bin/station-airplay
