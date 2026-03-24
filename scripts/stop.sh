#!/usr/bin/env bash
set +e

if [ -e ./scripts/pid ]
then
    kill $(cat ./scripts/pid) 2>/dev/null || true
fi
