#!/bin/sh

cd `dirname $0`
GOOS=linux GOARCH=amd64 go build && mv ./smppgate ./smppgate-linux-amd64
GOOS=linux GOARCH=386 go build && mv ./smppgate ./smppgate-linux-386
GOOS=windows GOARCH=386 go build && mv ./smppgate.exe ./smppgate-windows-386.exe
