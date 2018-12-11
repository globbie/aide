#!/usr/bin/env bash

ROOT_DIR="$(dirname "$PWD/$0")"

KNOWDY_SRC="${ROOT_DIR}/pkg/knowdy/knowdy"
KNOWDY_BUILD="${KNOWDY_SRC}/build"

[ -d "${KNOWDY_SRC}" ] || (echo "knowdy not found" && exit 1)

if [ ! -d "${KNOWDY_BUILD}" ]; then
    mkdir "${KNOWDY_BUILD}" && cd "${KNOWDY_BUILD}"
    cmake "${KNOWDY_SRC}" -DCMAKE_BUILD_TYPE=Release -DCMAKE_C_COMPILER=`go env CC`
    cd -
fi

make -C "${KNOWDY_BUILD}" -j knowdy_static gsl-parser-external glb-lib-external

