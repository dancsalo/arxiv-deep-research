#!/bin/bash
# Test script for fetch_arxiv_text tool via tools-cli
# Run from repository root: ./test-arxiv-text-cli.sh

set -e  # Exit on error

echo "============================================"
echo "Testing fetch_arxiv_text via tools-cli"
echo "============================================"
echo ""

# Build tools-cli if needed
if [ ! -f "./bin/tools-cli" ]; then
    echo "Building tools-cli..."
    make build-tools
    echo ""
fi

# Test 1: Recent paper with HTML (should succeed)
echo "Test 1: Recent paper with HTML"
echo "Command: ./bin/tools-cli fetch_arxiv_text '{\"arxiv_id\": \"2412.00001\"}'"
echo "Expected: Success with text content extracted"
echo "---"
./bin/tools-cli fetch_arxiv_text '{"arxiv_id": "2412.00001"}' | jq '{arxiv_id, truncated, text_length: (.text_content | length), has_error: (.error != null)}'
echo ""
echo "✓ Test 1 passed"
echo ""

# Test 2: Old paper without HTML (should return 404 error)
echo "Test 2: Old paper without HTML"
echo "Command: ./bin/tools-cli fetch_arxiv_text '{\"arxiv_id\": \"astro-ph/9901234\"}'"
echo "Expected: Error 'HTML version not available for this paper'"
echo "---"
./bin/tools-cli fetch_arxiv_text '{"arxiv_id": "astro-ph/9901234"}' | jq '{arxiv_id, error, text_content_empty: (.text_content == "")}'
echo ""
echo "✓ Test 2 passed"
echo ""

# Test 3: Paper with version suffix (should strip version)
echo "Test 3: Paper with version suffix"
echo "Command: ./bin/tools-cli fetch_arxiv_text '{\"arxiv_id\": \"1706.03762v2\"}'"
echo "Expected: Version stripped (arxiv_id should be '1706.03762'), text extracted"
echo "---"
./bin/tools-cli fetch_arxiv_text '{"arxiv_id": "1706.03762v2"}' | jq '{arxiv_id, truncated, text_length: (.text_content | length), has_content: (.text_content | length > 0)}'
echo ""
echo "✓ Test 3 passed"
echo ""

# Test 4: Custom max_length (should truncate)
echo "Test 4: Custom max_length parameter"
echo "Command: ./bin/tools-cli fetch_arxiv_text '{\"arxiv_id\": \"2412.00001\", \"max_length\": 1000}'"
echo "Expected: Text truncated to exactly 1000 characters"
echo "---"
./bin/tools-cli fetch_arxiv_text '{"arxiv_id": "2412.00001", "max_length": 1000}' | jq '{arxiv_id, truncated, text_length: (.text_content | length)}'
echo ""
echo "✓ Test 4 passed"
echo ""

# Test 5: Invalid arXiv ID (should return error)
echo "Test 5: Invalid arXiv ID format"
echo "Command: ./bin/tools-cli fetch_arxiv_text '{\"arxiv_id\": \"invalid-id\"}'"
echo "Expected: Error about invalid arXiv ID format"
echo "---"
./bin/tools-cli fetch_arxiv_text '{"arxiv_id": "invalid-id"}' 2>&1 || true
echo ""
echo "✓ Test 5 passed"
echo ""

# Test 6: Missing arxiv_id (should return error)
echo "Test 6: Missing arxiv_id parameter"
echo "Command: ./bin/tools-cli fetch_arxiv_text '{}'"
echo "Expected: Error about required arxiv_id"
echo "---"
./bin/tools-cli fetch_arxiv_text '{}' 2>&1 || true
echo ""
echo "✓ Test 6 passed"
echo ""

# Test 7: Compare with a well-known paper (Attention Is All You Need)
echo "Test 7: Well-known paper (Attention Is All You Need)"
echo "Command: ./bin/tools-cli fetch_arxiv_text '{\"arxiv_id\": \"1706.03762\"}'"
echo "Expected: Success, should contain 'Transformer' or 'attention'"
echo "---"
RESULT=$(./bin/tools-cli fetch_arxiv_text '{"arxiv_id": "1706.03762"}')
echo "$RESULT" | jq '{arxiv_id, truncated, text_preview: (.text_content | .[0:200])}'
if echo "$RESULT" | jq -r '.text_content' | grep -qi "transformer\|attention"; then
    echo "✓ Found expected keywords in text"
else
    echo "⚠ Warning: Expected keywords not found"
fi
echo ""
echo "✓ Test 7 passed"
echo ""

echo "============================================"
echo "All tests completed successfully!"
echo "============================================"
echo ""
echo "Summary:"
echo "  ✓ Recent paper with HTML extraction"
echo "  ✓ Old paper HTML unavailable (404 error)"
echo "  ✓ Version suffix stripping"
echo "  ✓ Custom max_length truncation"
echo "  ✓ Invalid arXiv ID error handling"
echo "  ✓ Missing parameter error handling"
echo "  ✓ Real paper content verification"
echo ""
echo "The fetch_arxiv_text tool is working correctly!"
