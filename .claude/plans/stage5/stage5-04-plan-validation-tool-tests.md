---
name: Research Plan Validation Tool - Test Plan
description: Test strategy for create_research_plan tool (DEFERRED)
type: test-plan
parent: stage5-04-plan-validation-tool
status: deferred
---

## Status: DEFERRED

This test plan corresponds to a deferred feature. Implement only if activation criteria in the main plan are met.

## Test Scope

### What We're Testing
- **Tool schema validation**: Input parameter parsing and validation
- **Section matching logic**: Regex-based keyword matching for three categories
- **Approval/rejection flow**: Correct status and message formatting
- **Edge cases**: Gaming attempts, multi-category matches, boundary conditions

### What We're NOT Testing
- Integration with agentic loop (requires running research demo)
- System prompt effectiveness (requires baseline evaluation)
- Agent behavior changes (requires A/B testing with real queries)

## Test Files

### Unit Tests: `tools/research/create_plan_test.go`

All tests use `NewResearchToolSet(&http.Client{})` constructor.

#### Test 1: Valid Plans with Exact Keywords
**Purpose**: Verify common section names pass validation

**Test cases**:
1. Exact names: "Seminal Papers", "Fundamental Techniques", "Survey Papers"
2. Alternative phrasing: "Pivotal Works", "Core Algorithms", "Review Literature"
3. Mixed case: "HIGHLY-CITED PAPERS", "how it works", "State of the Art"

**Expected**: All return `{"status":"approved", "sections":[...], "message":"...approved..."}`

**Verification**:
- Parse JSON response into `ResearchPlanResult`
- Assert `status == "approved"`
- Assert `len(sections) == 3`

#### Test 2: Missing Required Sections
**Purpose**: Verify rejection when plan lacks categories

**Test cases**:
1. Missing survey: only "Seminal Papers" + "Algorithms"
2. Missing techniques: only "Important Papers" + "Review Papers"
3. Missing seminal: only "Algorithms" + "Surveys"
4. Only one random section: "Random Section"

**Expected**: All return `{"status":"rejected", "sections":[...], "message":"...missing required sections: X, Y..."}`

**Verification**:
- Parse JSON response
- Assert `status == "rejected"`
- Assert `message` contains expected missing category keyword (case-insensitive)

#### Test 3: Input Validation Errors
**Purpose**: Verify parameter validation

**Test cases**:
1. Empty topic: `{"topic": "", "sections": [{"name":"Test", "description":"Test"}]}`
2. Empty sections array: `{"topic": "test", "sections": []}`
3. Missing sections field: `{"topic": "test"}`
4. Invalid JSON: `{topic: "missing quotes"}`

**Expected**:
- Cases 1-3: Return `{"error":"...", "recoverable":false}` (using `toolError` helper)
- Case 4: Handler returns error via `json.Unmarshal` failure

**Verification**:
- Result string contains `"error"` keyword
- Go error is nil (handler doesn't return non-nil errors)

#### Test 4: Section Matching Unit Tests
**Purpose**: Verify regex patterns work correctly

**Test cases** (testing each matcher function):

**Seminal section matching**:
- ✓ "seminal papers", "pivotal works", "highly-cited papers", "influential papers", "landmark study"
- ✗ "fundamental papers", "random section", "techniques"

**Techniques section matching**:
- ✓ "fundamental techniques", "core algorithm", "main method", "approach details", "mechanism", "architecture"
- ✗ "seminal papers", "survey", "tech stuff" (partial word)

**Survey section matching**:
- ✓ "survey papers", "review literature", "research landscape", "overview", "state of the art", "comparison study"
- ✗ "seminal papers", "algorithm", "random"

**Edge cases**:
- Word boundaries: "tech stuff" should NOT match techniques (regex requires full word)
- Multi-category: "review methods" matches survey (review) AND techniques (method)
- Case insensitive: "SEMINAL", "Seminal", "sEmInAl" all match

**Verification**:
- Call matcher function directly: `matchesSeminalSection(input)`
- Assert boolean result matches expected

#### Test 5: Anti-Gaming Cases
**Purpose**: Document known limitation - keyword stuffing

**Test cases**:
1. Single section with all keywords: "Seminal Fundamental Survey Papers"
2. Overlapping keywords across sections: "Important Review Papers" (seminal+survey) + "Core Algorithms" + "Research Landscape"

**Expected**:
- Case 1: Currently PASSES (known limitation - section matches all three categories)
- Case 2: PASSES (acceptable - multiple sections cover requirements)

**Verification**:
- Assert current behavior (approval)
- Log known limitation for future improvement

### Integration Tests (Manual)

#### Test 6: End-to-End with Research Demo
**Prerequisite**: Implement tool AND update system prompt (stage5-05)

**Steps**:
1. Build: `make build`
2. Run: `./bin/research-demo --query "transformers" --output test.json`
3. Inspect trace log at `test.json`

**Verification**:
- Turn 0 or 1 contains tool use: `"name": "create_research_plan"`
- Tool input has three sections with names matching patterns
- Tool result has `"status": "approved"`
- Subsequent turns contain search tool calls (agent proceeds after approval)

#### Test 7: Rejection and Retry Flow
**Prerequisite**: Same as Test 6

**Steps**:
1. Modify system prompt temporarily to request only 2 sections (remove "survey papers" requirement)
2. Run research demo
3. Inspect trace

**Verification**:
- Agent submits plan with 2 sections
- Tool returns `"status": "rejected"` with clear error message
- Agent submits revised plan in next turn
- Revised plan gets `"status": "approved"`
- Research proceeds

**Cleanup**: Revert system prompt changes

## Test Data

### Valid Plans
```json
{
  "topic": "transformers in NLP",
  "sections": [
    {"name": "Seminal Papers", "description": "Attention Is All You Need and foundational works"},
    {"name": "Fundamental Techniques", "description": "Self-attention mechanism, positional encoding"},
    {"name": "Survey Papers", "description": "Recent surveys on transformer applications"}
  ]
}
```

### Invalid Plans
```json
{
  "topic": "transformers in NLP",
  "sections": [
    {"name": "Random Section 1", "description": "Stuff"},
    {"name": "Random Section 2", "description": "More stuff"}
  ]
}
```

## Success Criteria

### Unit Tests
- ✓ All 5 test functions pass: `go test ./tools/research -v`
- ✓ No compilation errors
- ✓ Test coverage includes happy path, error cases, and edge cases

### Integration Tests (if implemented)
- ✓ Tool is callable (registered correctly)
- ✓ Agent calls tool in turn 0-1 (system prompt effectiveness)
- ✓ Approval flow allows research to proceed
- ✓ Rejection flow provides clear feedback and allows retry

## Known Test Gaps

1. **No test for sections with empty name/description**: Schema says required, but handler doesn't explicitly validate. Rely on JSON unmarshaling to enforce?
2. **No test for very long section names**: Potential DoS vector (100K character section name). Acceptable risk for v1.
3. **No test for Unicode/special characters**: e.g., "机器学习论文" (Chinese). Regexes use ASCII patterns but should handle gracefully.
4. **No performance test**: Regex compilation happens per request (inside matching functions). Should compile once globally. Not critical for validation tool with low call frequency.
5. **No concurrency test**: Handler doesn't maintain state, so thread-safe by design. No explicit test needed.

## Test Maintenance

### When to Update Tests
- **Keyword changes**: If matcher regexes are updated (adding/removing keywords), update Test 4 cases
- **Schema changes**: If tool input/output structure changes, update Test 1-3 parsing
- **New validation rules**: If tool enforces new requirements (e.g., min 2 papers per section), add new test function

### Test Philosophy
- **Unit tests verify correctness**: Does the code do what it claims?
- **Integration tests verify value**: Does the tool improve outcomes?
- **Known limitations are documented**: Tests acknowledge what CAN'T be enforced (gaming, false negatives)

## Baseline Evaluation (Required Before Implementation)

Before implementing ANY code, run baseline evaluation:

**Objective**: Determine if validation tool is needed

**Method**:
1. Implement stage5-01, stage5-02, stage5-03 tools (web search, GitHub search, PDF extraction)
2. Update system prompt with explicit planning instructions (NO validation tool)
3. Run research demo on 10-20 diverse topics
4. Manually score outputs on:
   - **Structure**: Does output have clear sections? (1-5 scale)
   - **Coverage**: Are seminal papers, techniques, and surveys all present? (yes/no per category)
   - **Quality**: Is research comprehensive and coherent? (1-5 scale)

**Decision**:
- If >70% of outputs have all three categories AND structure score >4 → **NO VALIDATION NEEDED**
- If <70% compliance OR structure score <4 → **IMPLEMENT TOOL** and re-evaluate

**Rationale**: Evidence-based feature development. Only build enforcement if prompt alone fails.
