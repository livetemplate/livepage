#!/usr/bin/env bash
# Simple script that outputs JSON data for exec toolbar test

cat << 'EOF'
[
  {"key": "hostname", "value": "test-machine"},
  {"key": "os", "value": "Darwin"},
  {"key": "uptime", "value": "42 days"}
]
EOF
