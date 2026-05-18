# Structured arXiv Parameters - Implementation Summary

**Date**: 2026-05-14  
**Goal**: Improve arXiv search fulfillment from 17.2% → 50%+  
**Approach**: Replace fragile heuristic inference with LLM-provided structured parameters

---

## Problem Statement

### Before: Heuristic Inference (17.2% Fulfillment)

**Issue 1: Sort Order Buries Old Papers**
- Query: "Attention Is All You Need"
- Heuristic: Detected 5 caps → exact phrase `ti:"Attention Is All You Need"`
- Problem: Default `sortBy=submittedDate` → newest first
- Result: ❌ Got 2026 papers, buried 2017 Vaswani paper

**Issue 2: Author Detection Fails**
- Query: "DDPM Ho"
- Heuristic: "DDPM" (caps) → detected as author, "Ho" (2 chars) → not detected
- Query: `au:DDPM+AND+(ti:Ho+OR+abs:Ho)`
- Result: ❌ 0 results (wrong author detection)

**Issue 3: Category Inference Too Strict**
- Query: "diffusion models"
- Heuristic: Needs 2+ keyword matches in category list
- Found: "model" (1 keyword) → insufficient
- Result: ❌ No category filter → mixed physics/biology/chemistry papers

**Issue 4: Generic Queries Fail**
- Query: "transformer"
- Heuristic: Single word, no category inference (< 2 keywords)
- Query: `(ti:transformer+OR+abs:transformer)` (no filter)
- Result: ❌ Returns astronomy papers

**Root Cause**: Heuristics are fragile. LLMs are better at parsing natural language than rule-based systems.

---

## Solution: Structured Parameters

**Paradigm Shift**: Stop guessing. Let the LLM parse and provide explicit parameters.

### New Parameters

```go
type SearchArxivParams struct {
    Query       string   `json:"query"`        // Search terms (required)
    Authors     []string `json:"authors"`      // Explicit author names
    Category    string   `json:"category"`     // Explicit arXiv category
    ExactPhrase bool     `json:"exact_phrase"` // Force exact title match
    MaxResults  int      `json:"max_results"`
    SortBy      string   `json:"sort_by"`      // submittedDate, relevance
    SortOrder   string   `json:"sort_order"`   // ascending, descending
}
```

### Query Construction

**Structured Mode** (when any structured param provided):
```go
func buildStructuredQuery(query, authors, category, exactPhrase) {
    var parts []string
    
    if category != "" {
        parts = append("cat:" + category)
    }
    
    if len(authors) > 0 {
        parts = append("(au:Author1+AND+au:Author2)")
    }
    
    if exactPhrase {
        parts = append("ti:\"" + query + "\"")
    } else {
        parts = append("(ti:term1+OR+abs:term1)+AND+(ti:term2+OR+abs:term2)")
    }
    
    return strings.Join(parts, "+AND+")
}
```

**Inference Mode** (fallback when no structured params):
- Old heuristic code preserved
- Backward compatibility maintained

---

## Implementation Timeline

### Phase 1: Add Structured Parameters ✅ COMPLETE

**Code Changes**:
1. `tools/research/handlers.go`:
   - Added `Authors`, `Category`, `ExactPhrase` fields to params struct
   - Implemented `buildStructuredQuery()` function
   - Added mode detection: `useStructured = Authors != nil || Category != "" || ExactPhrase`
   - Smart defaults: exact_phrase → relevance sort, otherwise → submittedDate sort
   - Preserved inference fallback

2. `tools/research/definitions.go`:
   - Added `authors` (array), `category` (string), `exact_phrase` (boolean) to tool schema
   - Updated description with structured examples

3. `cmd/tools-cli/commands.go`:
   - Added CLI flags: `--authors`, `--category`, `--exact-phrase`, `--sort-by`, `--sort-order`
   - Fixed flag parsing order (extract query first)
   - CSV parsing for comma-separated authors

**Test Results**: 5/5 test cases passed
1. ✅ Vaswani paper found with `--exact-phrase --sort-by=relevance`
2. ✅ Kipf GCN papers found with `--authors=Kipf --category=cs.LG`
3. ✅ DDPM paper found with `--authors=Ho --exact-phrase`
4. ✅ CV diffusion papers filtered with `--category=cs.CV`
5. ✅ Backward compatibility works (inference mode without flags)

**Documentation**: `STRUCTURED_PARAMS_TEST_RESULTS.md`

### Phase 2: Update System Prompt ✅ COMPLETE

**Prompt Changes** (`cmd/research-demo/prompts/variant-b-metacognitive.txt`):

1. **Updated Tool List**:
   ```
   - search_arxiv: Search arXiv for academic preprints (supports structured params: authors, category, exact_phrase, sort_by)
   ```

2. **Added 4 Usage Patterns**:
   - Pattern 1: Find Specific Paper (exact_phrase + relevance)
   - Pattern 2: Find Papers by Author on Topic (authors + category)
   - Pattern 3: Recent Papers on Topic (category + submittedDate)
   - Pattern 4: Topic Search in Specific Domain (category only)

3. **Added Category Reference Table**:
   - cs.LG: Machine Learning
   - cs.CV: Computer Vision
   - cs.CL: Natural Language Processing
   - cs.AI: Artificial Intelligence
   - cs.RO: Robotics
   - stat.ML: Statistics/ML

4. **Added When-to-Use Guidance**:
   - When to use `exact_phrase=true`
   - When to use `category` filter
   - When to use `authors` parameter

**Teaching Strategy**: Pattern-based examples showing BEST vs BAD approaches

**Documentation**: `PHASE_2_PROMPT_UPDATE.md`

### Phase 3: Evaluate and Remove Inference 📋 TODO

**Evaluation Plan**:
1. Run 3 diverse queries:
   - "graph neural networks"
   - "diffusion models for image generation"
   - "reinforcement learning from human feedback"

2. Extract datasets from traces:
   ```bash
   cd analysis_tools/dataset_extractor
   ./tool_eval_dataset_extractor ../../.traces/demo-*.json
   ```

3. Judge fulfillment:
   ```bash
   cd ../..
   ./llm_rationale_judge ./.traces/demo-*_dataset.json
   ```

4. Compare metrics:
   - Before: 17.2% overall, ~20% search_arxiv
   - Target: >50% overall, >70% search_arxiv

**If Target Met** (>45% overall):
- Remove inference code:
  - `detectRealAuthorNames()`
  - `inferArxivCategory()`
  - Capitalization detection
  - All keyword lists
- Simplify to single `buildStructuredQuery()` path
- Update tests

**If Target Not Met**:
- Analyze failure modes
- Improve prompt examples
- Consider adding explicit instruction: "ALWAYS use structured parameters"
- May need Plan B: Query formulation sidecar tool

---

## Key Benefits

### 1. Eliminates Fragile Heuristics ✅
- ❌ Before: Guess if "DDPM" is author or acronym
- ✅ After: LLM provides `authors=["Ho"]`

### 2. Explicit Sort Control ✅
- ❌ Before: Default sort buried old papers
- ✅ After: `exact_phrase` → auto relevance sort

### 3. Category Filtering Works ✅
- ❌ Before: "diffusion" → no category (1 keyword)
- ✅ After: LLM provides `category="cs.CV"`

### 4. Backward Compatible ✅
- Old queries still work (inference fallback)
- New queries use structured params
- Gradual migration

### 5. More Expressive ✅
- "Find exact paper X" → `exact_phrase=true`
- "Search in ML only" → `category="cs.LG"`
- "Find by author Y" → `authors=["Y"]`

### 6. Easier to Debug ✅
- Structured input shows intent explicitly
- No hidden heuristic logic
- Clear query construction

### 7. Maintainable ✅
- No keyword lists to update
- No detection thresholds to tune
- No edge cases for names/acronyms

---

## Expected Impact

| Metric | Before | Phase 1 | Phase 2 | Phase 3 | Notes |
|--------|--------|---------|---------|---------|-------|
| Overall Fulfillment | 17.2% | 35-45% | 50-60% | 55-65% | Phase 1: code works, Phase 2: agent learns, Phase 3: remove inference |
| search_arxiv Fulfillment | ~20% | 60-70% | 70-80% | 75-85% | Structured params work immediately |
| Exact Phrase Success | 0% | 100% | 100% | 100% | Relevance sort fixes old papers |
| Author Search Success | ~30% | 90% | 95% | 95% | No heuristic detection |
| Category Filter Success | ~40% | 85% | 90% | 90% | Explicit categories |
| Wrong-Domain Results | ~60% | <10% | <5% | <5% | Category filtering |

---

## Files Modified

### Phase 1 (Code)
- `tools/research/handlers.go` - Added structured query builder, mode detection, smart defaults
- `tools/research/definitions.go` - Added authors/category/exact_phrase parameters to tool schema
- `cmd/tools-cli/commands.go` - Added CLI flags for structured parameters

### Phase 2 (Prompt)
- `cmd/research-demo/prompts/variant-b-metacognitive.txt` - Added structured parameter guidance, patterns, examples, category table

### Documentation
- `STRUCTURED_ARXIV_PROPOSAL.md` - Original proposal and design
- `ARXIV_TEST_RESULTS.md` - Initial testing that discovered sort order issue
- `STRUCTURED_PARAMS_TEST_RESULTS.md` - Phase 1 test results
- `PHASE_2_PROMPT_UPDATE.md` - Phase 2 changes and strategy
- `STRUCTURED_PARAMS_IMPLEMENTATION_SUMMARY.md` - This file

---

## Testing Commands

### CLI Testing (Phase 1)
```bash
# Exact phrase search
./tools-cli search-arxiv "Attention Is All You Need" --exact-phrase --sort-by=relevance

# Author + category search
./tools-cli search-arxiv "graph convolutional networks" --authors=Kipf --category=cs.LG

# Topic + category search
./tools-cli search-arxiv "diffusion models" --category=cs.CV

# Backward compatibility
./tools-cli search-arxiv "graph neural networks"
```

### Agent Testing (Phase 2)
```bash
# Run research demo
./research-demo -query "graph neural networks" -max-turns 2 -trace-dir .traces

# Check if agent uses structured params
cat .traces/demo-*.json | jq '.turns[].actions[] | select(.tool=="search_arxiv") | .input'

# Should see: exact_phrase, authors, category fields
```

### Evaluation (Phase 3)
```bash
# Run 3 test queries
./research-demo -query "graph neural networks" -max-turns 3 -trace-dir .traces
./research-demo -query "diffusion models for image generation" -max-turns 3 -trace-dir .traces
./research-demo -query "reinforcement learning from human feedback" -max-turns 3 -trace-dir .traces

# Extract datasets
cd analysis_tools/dataset_extractor
./tool_eval_dataset_extractor ../../.traces/demo-*.json

# Judge fulfillment
cd ../..
./llm_rationale_judge ./.traces/demo-*_dataset.json

# Compare: Before (17.2%) vs After (target: >50%)
```

---

## Conclusion

**Phase 1 ✅**: Structured parameters implemented and tested. All 5 test cases pass.

**Phase 2 ✅**: System prompt updated with patterns, examples, and category table.

**Phase 3 📋**: Awaiting agent test to verify prompt teaches structured parameter usage correctly.

**Expected Outcome**: 
- If agent uses structured params → 50-60% fulfillment (3x improvement)
- If agent ignores params → improve prompt or add explicit instructions
- If >45% after Phase 2 → remove inference code in Phase 3

**Key Insight**: Moving intelligence from tool (inference) to agent (LLM) eliminates fragile heuristics and leverages LLM's natural language understanding.
