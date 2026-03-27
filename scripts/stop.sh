#!/usr/bin/env bash
set +e

if [ -e ./.backend/pid ]
then
    kill $(cat ./.backend/pid) 2>/dev/null || true
fi
