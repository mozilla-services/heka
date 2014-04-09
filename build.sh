#!/usr/bin/env bash

# if the environment has been setup before clean it up
if [ $GOBIN ]; then
    PATH=$(echo $PATH | sed -e "s;\(^$GOBIN:\|:$GOBIN$\|:$GOBIN\(:\)\);\2;g")
fi

BUILD_DIR=$PWD/build
export CTEST_OUTPUT_ON_FAILURE=1
export GOPATH=$BUILD_DIR/heka
export GOBIN=$GOPATH/bin
export PATH=$PATH:$GOBIN
mkdir -p $BUILD_DIR
cd $BUILD_DIR
cmake -DCMAKE_BUILD_TYPE=release ..
make
