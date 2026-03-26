@echo off

if not exist logs mkdir logs

start /B neo-pkg-llm.exe > logs\stdout.log 2> logs\stderr.log
