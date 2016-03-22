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

cd lib/libgit2-0.22.0

cmake -DCMAKE_INSTALL_PREFIX=/usr .
make
make install

mkdir -p /tmp/gobuild/src
mkdir -p /tmp/gobuild/pkg
mkdir -p /tmp/gobuild/bin
mkdir -p /usr/share/go-gitlab
export GOPATH=/tmp/gobuild

go get
go build

cp -rf templates /usr/share/go-gitlab
cp -rf githooks.conf /etc/
