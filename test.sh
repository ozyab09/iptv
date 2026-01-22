#!/bin/bash

# Test script for local M3U filter scenarios
# This script runs different scenarios for the M3U filter pipeline

echo "Starting local tests for M3U Filter Pipeline..."

# Function to run a test scenario
run_test() {
    local scenario_name="$1"
    local dry_run="$2"
    local description="$4"

    echo
    echo "==========================================="
    echo "Running test: $scenario_name"
    echo "Description: $description"
    echo "Dry run mode: $dry_run"
    echo "==========================================="

    # Set environment variables based on test scenario
    if [ "$dry_run" = "true" ]; then
        export DRY_RUN=true
        echo "Running in dry-run mode"
    else
        unset DRY_RUN
        echo "Running in normal mode"
    fi

    # Install dependencies and run the organized M3U filter script
    pip install -e .
    cd src && python run_filter.py

    local exit_code=$?

    if [ $exit_code -eq 0 ]; then
        echo "✅ Test '$scenario_name' PASSED"
    else
        echo "❌ Test '$scenario_name' FAILED with exit code: $exit_code"
    fi

    echo "==========================================="
    echo
}

# Run all test scenarios
run_test "Pull Request Test" "true" "" "Testing dry-run mode for pull requests"
run_test "Scheduled Pipeline Test" "false" "" "Testing normal execution for scheduled pipelines"
run_test "Main Branch Test" "false" "" "Testing normal execution for main branch pushes"

echo "All tests completed!"