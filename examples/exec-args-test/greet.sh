#!/usr/bin/env bash

# Handle --help flag
if [ "$1" = "--help" ] || [ "$1" = "-h" ]; then
    cat << 'EOF'
Usage: greet.sh [options]

Options:
  --name NAME      The name to greet (default: World)
  --count N        Number of times to repeat (default: 1)
  --uppercase      Output in uppercase (default: false)
  --help           Show this help message
EOF
    exit 0
fi

# Parse arguments
name="World"
count=1
uppercase="false"

while [[ $# -gt 0 ]]; do
    case $1 in
        --name) name="$2"; shift 2 ;;
        --count) count="$2"; shift 2 ;;
        --uppercase) uppercase="$2"; shift 2 ;;
        *) shift ;;
    esac
done

# Generate JSON output
echo "["
for i in $(seq 1 $count); do
    if [ "$uppercase" = "true" ]; then
        greeting=$(echo "HELLO, $name!" | tr '[:lower:]' '[:upper:]')
    else
        greeting="Hello, $name!"
    fi

    if [ $i -eq $count ]; then
        echo "  {\"index\": $i, \"message\": \"$greeting\"}"
    else
        echo "  {\"index\": $i, \"message\": \"$greeting\"},"
    fi
done
echo "]"
