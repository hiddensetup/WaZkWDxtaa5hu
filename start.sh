#!/bin/sh

# kill prev copy if exists
pkill -e -f w1


# start a new one
./w1 &
