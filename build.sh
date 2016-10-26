#!/bin/bash - 
#===============================================================================
#
#          FILE: build.sh
# 
#         USAGE: ./build.sh 
# 
#   DESCRIPTION: 
# 
#       OPTIONS: ---
#  REQUIREMENTS: ---
#          BUGS: ---
#         NOTES: ---
#        AUTHOR: Stan P. (@vagner), root.vagner@gmail.com
#  ORGANIZATION: 
#       CREATED: 24.03.2015 11:29
#      REVISION:  ---
#===============================================================================

set -o nounset                              # Treat unset variables as an error

pushd lib/libgit2-0.22.0

cmake -DCMAKE_INSTALL_PREFIX=/usr .
make
make install

export GOPATH=/tmp/gobuild
mkdir -p ${GOPATH}/src ${GOPATH}/pkg ${GOPATH}/bin /usr/share/go-gitlab

popd

go get
go build

cp -rf templates /usr/share/go-gitlab
cp -rf githooks.conf /etc/
