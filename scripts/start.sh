#!/usr/bin/env bash
set -e

mkdir -p ./logs

echo $$ > ./scripts/pid
exec ./neo-pkg-llm > ./logs/stdout.log 2> ./logs/stderr.log
