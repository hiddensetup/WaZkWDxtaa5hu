#!/bin/sh

# kill prev copy if exists
pkill -e -f wam


# start a new one
./wam &
