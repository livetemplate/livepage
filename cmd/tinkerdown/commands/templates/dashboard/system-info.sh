#!/usr/bin/env bash
# System information as JSON
# Works on macOS and Linux

if command -v df &> /dev/null; then
    df -h 2>/dev/null | awk 'NR>1 {print "{\"filesystem\":\""$1"\",\"size\":\""$2"\",\"used\":\""$3"\",\"available\":\""$4"\",\"use_percent\":\""$5"\",\"mount\":\""$6"\"}"}' | jq -s '.'
else
    echo '[]'
fi
