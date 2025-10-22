#!/bin/bash

# Simple Message Queue Test Runner
# Runs all tests in the project with detailed output

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Function to print colored output
print_status() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

print_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Function to run tests in a specific directory
run_test_suite() {
    local test_dir=$1
    local suite_name=$2

    print_status "Running $suite_name tests..."

    if go test -v "./$test_dir" -timeout 30s; then
        print_success "$suite_name tests passed"
        return 0
    else
        print_error "$suite_name tests failed"
        return 1
    fi
}

# Main test execution
main() {
    echo "=========================================="
    echo "          Simple Message Queue Test Runner"
    echo "=========================================="
    echo

    # Check if Go is installed
    if ! command -v go &> /dev/null; then
        print_error "Go is not installed or not in PATH"
        exit 1
    fi

    print_status "Go version: $(go version)"
    echo

    # Initialize variables to track results
    total_suites=0
    passed_suites=0
    failed_suites=()

    # Test suites to run (test_dir:suite_name pairs)
    test_suites=(
        "unit/api:API Handler"
        "unit/storage/sqlite:SQLite Storage"
    )

    # Run each test suite
    for suite in "${test_suites[@]}"; do
        test_dir="${suite%%:*}"
        suite_name="${suite#*:}"
        total_suites=$((total_suites + 1))

        echo "----------------------------------------"
        if run_test_suite "$test_dir" "$suite_name"; then
            passed_suites=$((passed_suites + 1))
        else
            failed_suites+=("$suite_name")
        fi
        echo
    done

    # Run all tests together for coverage
    echo "----------------------------------------"
    print_status "Running all tests together..."
    if go test ./unit/... -race -timeout 60s; then
        print_success "All tests completed"
    else
        print_warning "Some tests failed in combined run"
    fi
    echo

    # Generate test coverage report
    echo "----------------------------------------"
    print_status "Generating test coverage report..."
    if go test ./unit/... -coverprofile=coverage.out -timeout 60s; then
        if command -v go &> /dev/null; then
            coverage=$(go tool cover -func=coverage.out | grep total | awk '{print $3}')
            print_success "Test coverage: $coverage"

            # Generate HTML coverage report
            go tool cover -html=coverage.out -o coverage.html
            print_success "HTML coverage report generated: coverage.html"
        fi
    else
        print_warning "Coverage report generation failed"
    fi
    echo

    # Summary
    echo "=========================================="
    echo "              Test Summary"
    echo "=========================================="
    echo "Total test suites: $total_suites"
    echo "Passed: $passed_suites"
    echo "Failed: $((total_suites - passed_suites))"

    if [ ${#failed_suites[@]} -gt 0 ]; then
        echo
        print_error "Failed test suites:"
        for suite in "${failed_suites[@]}"; do
            echo "  - $suite"
        done
        echo
        exit 1
    else
        echo
        print_success "All test suites passed!"
        echo
        exit 0
    fi
}

# Run specific test suite if argument provided
if [ $# -eq 1 ]; then
    case $1 in
        "api")
            run_test_suite "unit/api" "API Handler"
            ;;
        "storage")
            run_test_suite "unit/storage/sqlite" "SQLite Storage"
            ;;
        "coverage")
            print_status "Running tests with coverage..."
            go test ./unit/... -coverprofile=coverage.out -timeout 60s
            go tool cover -func=coverage.out
            go tool cover -html=coverage.out -o coverage.html
            print_success "Coverage report generated: coverage.html"
            ;;
        "clean")
            print_status "Cleaning test artifacts..."
            rm -f coverage.out coverage.html
            print_success "Test artifacts cleaned"
            ;;
        "help"|"-h"|"--help")
            echo "Usage: $0 [option]"
            echo
            echo "Options:"
            echo "  api       Run only API tests"
            echo "  storage   Run only storage tests"
            echo "  coverage  Run tests with coverage report"
            echo "  clean     Clean test artifacts"
            echo "  help      Show this help message"
            echo
            echo "Run without arguments to run all tests"
            ;;
        *)
            print_error "Unknown option: $1"
            echo "Run '$0 help' for usage information"
            exit 1
            ;;
    esac
else
    main
fi