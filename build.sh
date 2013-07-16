#!/bin/bash

# if the environment has been setup before clean it up
if [ $GOBIN ]; then
    PATH=$(echo $PATH | sed -e "s;\(^$GOBIN:\|:$GOBIN$\|:$GOBIN\(:\)\);\2;g")
fi

BUILD_DIR=$PWD/build
export GOPATH=$BUILD_DIR/heka
export GOBIN=$GOPATH/bin
export PATH=$PATH:$GOBIN
mkdir -p $BUILD_DIR
cd $BUILD_DIR
cmake ..
make

