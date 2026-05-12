#!/bin/bash
# Quick test script for fetch_arxiv_text tool
# Run from repository root: ./test-arxiv-text-simple.sh

echo "Building tools-cli..."
make build-tools

echo ""
echo "=== Test 1: Recent paper (should succeed) ==="
./bin/tools-cli fetch_arxiv_text '{"arxiv_id": "2412.00001"}'

echo ""
echo "=== Test 2: Old paper (should show 404 error) ==="
./bin/tools-cli fetch_arxiv_text '{"arxiv_id": "astro-ph/9901234"}'

echo ""
echo "=== Test 3: Version suffix (should strip v2) ==="
./bin/tools-cli fetch_arxiv_text '{"arxiv_id": "1706.03762v2"}'

echo ""
echo "=== Test 4: Custom length (should truncate to 1000) ==="
./bin/tools-cli fetch_arxiv_text '{"arxiv_id": "2412.00001", "max_length": 1000}'

echo ""
echo "Done! All tests completed."
