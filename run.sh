#!/bin/sh

# kill prev copy if exists
pkill -e -f wa


# start a new one
./wa &
