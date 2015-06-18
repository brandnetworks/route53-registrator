#!/usr/bin/env bash

docker events | grep --line-buffered $1 | ./trigger.sh

