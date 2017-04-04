#!/bin/sh

go build goconfd.go

GOOS=linux GOARCH=amd64 go build -o goconfd-linux-amd64 goconfd.go
GOOS=linux GOARCH=arm go build -o goconfd-linux-arm goconfd.go
GOOS=linux GOARCH=386 go build -o goconfd-linux-386 goconfd.go

