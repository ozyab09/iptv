#!/bin/bash
# Script to load environment variables from .env file and run the filter

# Load environment variables from .env file
if [ -f .env ]; then
    export $(grep -v '^#' .env | xargs)
fi

# Run the filter from the current directory (not changing to src)
python src/run_filter.py