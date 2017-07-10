#!/bin/bash

set -v

dir="$(dirname "$0")"
. "${dir}/install-go.sh"

set -e

cd ${GOPATH}/src/github.com/skydive-project/skydive

make fmt
make vet

# check if unused package are listed in the vendor directory
if [ -n "$(${GOPATH}/bin/govendor list +u)" ]; then
   echo "You must remove these usused packages :"
   ${GOPATH}/bin/govendor list +u
   exit 1
fi

make lint
nbnotcomment=$(grep '"linter":"golint"' lint.json | grep 'should have comment or be unexported' | wc -l)
if $nbnotcomment -gt 134
   cat lint.json
   echo "===> You should comment you code <==="
   exit 1
fi
