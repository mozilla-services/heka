#!/usr/bin/env bash

# set up our environment
. ./env.sh

NUM_JOBS=${NUM_JOBS:-1}

# build heka
mkdir -p $BUILD_DIR
cd $BUILD_DIR
cmake -DCMAKE_BUILD_TYPE=release ..
make -j $NUM_JOBS
