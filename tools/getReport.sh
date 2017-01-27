#!/bin/sh

cd `dirname $0`
listen=`cat ../config.json | grep -Po '"'"listen"'"\s*:\s*"\K([^"]*)'`
projectPath=`cat ../config.json | grep -Po '"'"projectPath"'"\s*:\s*"\K([^"]*)'`
forwardSecret=`cat ../config.json | grep -Po '"'"forwardSecret"'"\s*:\s*"\K([^"]*)'`

curl --fail --silent -H "X-Forward-Secret: $forwardSecret" http://${listen}${projectPath}/dayReport?date=`date +%Y-%m-%d`
