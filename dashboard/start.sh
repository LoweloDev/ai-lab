#!/usr/bin/env bash
# Startet das AI-Lab Benchmark-Dashboard auf http://127.0.0.1:8100
cd "$(dirname "$0")"
exec python3 server.py "$@"
