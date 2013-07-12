#!/bin/bash

# if the environment has been setup before clean it up
if [ $GOPATH ]; then
    PATH=$(echo $PATH | sed -e "s;\(^$GOPATH/bin:\|:$GOPATH/bin$\|:$GOPATH/bin\(:\)\);\2;g")
fi

BUILD_DIR=$PWD/build
export GOPATH=$BUILD_DIR/heka
export PATH=$PATH:$GOPATH/bin
mkdir -p $BUILD_DIR
cd $BUILD_DIR
cmake ..
make

