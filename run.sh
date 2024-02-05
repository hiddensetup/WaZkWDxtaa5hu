#!/bin/sh

# kill prev copy if exists
pkill -e -f wap


# start a new one
./wap &
