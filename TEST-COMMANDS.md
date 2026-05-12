# Testing fetch_arxiv_text Tool

## Quick Start

### Option 1: Comprehensive Test Script (with validation)
```bash
./test-arxiv-text-cli.sh
```
This script runs 7 tests with validation and formatted output.

### Option 2: Simple Test Script (raw output)
```bash
./test-arxiv-text-simple.sh
```
This script runs 4 basic tests showing raw JSON output.

---

## Manual Test Commands

First, build the tools-cli:
```bash
make build-tools
```

### Test 1: Recent paper with HTML (should succeed)
```bash
./bin/tools-cli fetch_arxiv_text '{"arxiv_id": "2412.00001"}'
```

**Expected output:**
```json
{
  "arxiv_id": "2412.00001",
  "text_content": "...extracted text...",
  "truncated": false
}
```

### Test 2: Old paper without HTML (404 error)
```bash
./bin/tools-cli fetch_arxiv_text '{"arxiv_id": "astro-ph/9901234"}'
```

**Expected output:**
```json
{
  "arxiv_id": "astro-ph/9901234",
  "text_content": "",
  "truncated": false,
  "error": "HTML version not available for this paper"
}
```

### Test 3: Paper with version suffix (strips version)
```bash
./bin/tools-cli fetch_arxiv_text '{"arxiv_id": "1706.03762v2"}'
```

**Expected output:**
```json
{
  "arxiv_id": "1706.03762",
  "text_content": "...Attention Is All You Need...",
  "truncated": false
}
```
Note: `arxiv_id` should be "1706.03762" (without "v2")

### Test 4: Custom max_length (truncation)
```bash
./bin/tools-cli fetch_arxiv_text '{"arxiv_id": "2412.00001", "max_length": 1000}'
```

**Expected output:**
```json
{
  "arxiv_id": "2412.00001",
  "text_content": "...exactly 1000 characters...",
  "truncated": true
}
```

### Test 5: Invalid arXiv ID (error)
```bash
./bin/tools-cli fetch_arxiv_text '{"arxiv_id": "invalid-id"}'
```

**Expected:** Error about invalid arXiv ID format

### Test 6: Missing arxiv_id parameter (error)
```bash
./bin/tools-cli fetch_arxiv_text '{}'
```

**Expected:** Error about required arxiv_id

### Test 7: Different arXiv ID formats

All of these should work:
```bash
# New format
./bin/tools-cli fetch_arxiv_text '{"arxiv_id": "2301.00001"}'

# With arXiv prefix
./bin/tools-cli fetch_arxiv_text '{"arxiv_id": "arXiv:2301.00001"}'

# With version
./bin/tools-cli fetch_arxiv_text '{"arxiv_id": "2301.00001v2"}'

# Old format
./bin/tools-cli fetch_arxiv_text '{"arxiv_id": "astro-ph/9901234"}'

# Full URL
./bin/tools-cli fetch_arxiv_text '{"arxiv_id": "https://arxiv.org/abs/2301.00001"}'
```

---

## Testing with jq (formatted output)

If you have `jq` installed, you can format the output:

```bash
./bin/tools-cli fetch_arxiv_text '{"arxiv_id": "2412.00001"}' | jq
```

Show only specific fields:
```bash
./bin/tools-cli fetch_arxiv_text '{"arxiv_id": "2412.00001"}' | jq '{arxiv_id, truncated, length: (.text_content | length)}'
```

Check for errors:
```bash
./bin/tools-cli fetch_arxiv_text '{"arxiv_id": "astro-ph/9901234"}' | jq '{arxiv_id, error}'
```

---

## Expected Behavior

### Success Cases
- **Status:** Returns JSON with `arxiv_id`, `text_content`, and `truncated` fields
- **text_content:** Non-empty string with extracted text from arXiv HTML
- **truncated:** `true` if content exceeded max_length, `false` otherwise
- **Rate limiting:** 3 seconds between requests (per arXiv TOS)

### Error Cases
- **404 (HTML not available):** Returns partial result with `error` field and empty `text_content`
- **Invalid ID:** Returns error about invalid arXiv ID format
- **Network errors:** Returns error with `recoverable: true`
- **Missing parameter:** Returns error about required field

### Truncation
- **Default:** 25000 characters (if not specified)
- **Maximum:** 25000 characters (cannot exceed)
- **Custom:** Can specify any value up to 25000

---

## Notes

- The tool fetches HTML from `https://arxiv.org/html/{arxiv_id}`
- Not all papers have HTML versions (especially older papers)
- Text extraction uses the go-readability library
- Version suffixes (v1, v2, etc.) are automatically stripped
- Rate limiting enforces 3 seconds between requests
