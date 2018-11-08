#!/bin/sh
set -e

go get -u github.com/go-chi/chi
go get -u github.com/go-chi/chi/middleware
go get -u github.com/go-chi/render
go get -u github.com/maxkondr/porta-sip-anonymizer

CGO_ENABLED=0 GOOS=linux go build -ldflags '-w -s' -a -o ba-logs-anonymizer .
