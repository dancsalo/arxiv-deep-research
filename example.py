#!/usr/bin/env python3

from __future__ import annotations

import json
import logging
import os
from pathlib import Path
import re
import textwrap
from dataclasses import dataclass, asdict
from typing import List, Optional

import anyio
import httpx
from bs4 import BeautifulSoup
from rich.console import Console
from rich.logging import RichHandler

from src.tools.arxiv_search import SearchArxivInput, search_arxiv_tool
from src.tools.memory_store import store_report as store_report_to_db
from src.tools.memory_tools import memory_mcp_server

from claude_agent_sdk import (
    AssistantMessage,
    ClaudeAgentOptions,
    ClaudeSDKClient,
    ResultMessage,
    TextBlock,
    ToolResultBlock,
    UserMessage,
    create_sdk_mcp_server,
    query,
    tool,
)
from dotenv import load_dotenv
from langfuse import Langfuse

load_dotenv()
langfuse = Langfuse()

# -----------------------------
# Logging setup
# -----------------------------

os.makedirs("logs", exist_ok=True)

console = Console()
log = logging.getLogger("arxiv_research")
log.setLevel(logging.DEBUG)

# Rich console handler (INFO+)
rich_handler = RichHandler(
    console=console,
    show_time=True,
    show_path=False,
    markup=True,
    rich_tracebacks=True,
)
rich_handler.setLevel(logging.INFO)

# File handler (DEBUG+)
file_handler = logging.FileHandler("logs/research.log", encoding="utf-8")
file_handler.setLevel(logging.DEBUG)
file_handler.setFormatter(
    logging.Formatter("%(asctime)s | %(levelname)-8s | %(message)s", datefmt="%Y-%m-%d %H:%M:%S")
)

log.addHandler(rich_handler)
log.addHandler(file_handler)


# -----------------------------
# Data models
# -----------------------------

@dataclass
class Paper:
    title: str
    authors: List[str]
    summary: str
    pdf_url: str
    entry_id: str
    published: str


@dataclass
class WebResult:
    title: str
    url: str
    snippet: str
    content: Optional[str] = None


# -----------------------------
# arXiv search skill (Instructions + In-Process SDK Tool)
# -----------------------------
#
# Skill = Instructions + Tool
#   Instructions: concise system prompt telling Claude how to search
#   Tool: in-process MCP tool via ClaudeSDKClient + @tool decorator
#
# Claude autonomously extracts search params from the NL request and calls the tool.
#

SKILLS_DIR = Path(__file__).parent / "src" / "skills"

# System prompt for the arXiv skill agent
_ARXIV_SKILL_PROMPT = (
    "You are an arXiv search agent. You have a search_arxiv tool. "
    "Given a research request: (1) extract concise keyword queries, drop instruction verbs, "
    "(2) pick arXiv categories (cs.LG, cs.AI, cs.CL, etc.), "
    "(3) use sort_by=submittedDate for recent/latest, otherwise relevance, "
    "(4) call search_arxiv. After calling the tool, output ONLY the raw JSON "
    "from the tool result. No summaries, no markdown, no commentary."
)


@tool(
    "search_arxiv",
    "Search arXiv for academic papers by query, category, result count, and sort order.",
    {"query": str, "categories": list, "max_results": int, "sort_by": str},
)
async def _search_arxiv_tool_handler(args):
    search_input = SearchArxivInput(
        query=args["query"],
        categories=args.get("categories", []),
        max_results=args.get("max_results", 10),
        sort_by=args.get("sort_by", "relevance"),
    )
    result = search_arxiv_tool(search_input)
    return {"content": [{"type": "text", "text": result.model_dump_json()}]}


_arxiv_mcp_server = create_sdk_mcp_server(name="arxiv", tools=[_search_arxiv_tool_handler])


async def run_arxiv_skill(user_input: str) -> List[Paper]:
    """
    arXiv search skill: single agent call with in-process SDK tool.

    Claude reads instructions, formulates the right query, calls the
    search_arxiv tool via in-process MCP, and returns papers.
    """
    options = ClaudeAgentOptions(
        system_prompt=_ARXIV_SKILL_PROMPT,
        max_turns=4,
        permission_mode="bypassPermissions",
        mcp_servers={"arxiv": _arxiv_mcp_server},
    )

    text_chunks: List[str] = []
    tool_results: List[str] = []
    try:
        async with ClaudeSDKClient(options=options) as client:
            await client.connect(user_input)
            async for msg in client.receive_messages():
                if isinstance(msg, AssistantMessage):
                    for block in msg.content:
                        if isinstance(block, TextBlock):
                            text_chunks.append(block.text)
                elif isinstance(msg, UserMessage):
                    for block in msg.content:
                        if isinstance(block, ToolResultBlock) and block.content:
                            if isinstance(block.content, str):
                                tool_results.append(block.content)
                            elif isinstance(block.content, list):
                                for item in block.content:
                                    if isinstance(item, dict) and item.get("type") == "text":
                                        tool_results.append(item["text"])
                if isinstance(msg, ResultMessage):
                    break
    except Exception as exc:
        log.warning("In-process skill error (will try fallback): %s", exc)

    # Parse papers from tool results (each is a separate JSON blob)
    papers: List[Paper] = []
    if tool_results:
        for tr in tool_results:
            parsed = try_parse_json(tr)
            papers.extend(_extract_papers(parsed))
    else:
        raw = "\n".join(text_chunks).strip()
        log.debug("arXiv skill text output: %s", raw[:500])
        parsed = try_parse_json(raw)
        papers = _extract_papers(parsed)

    # Deduplicate by entry_id
    seen = set()
    unique_papers: List[Paper] = []
    for p in papers:
        if p.entry_id not in seen:
            seen.add(p.entry_id)
            unique_papers.append(p)
    papers = unique_papers

    if not papers:
        log.error("Skill returned no papers, something is wrong.")
        exit()

    log.info("Found [green]%d[/] papers via arXiv skill", len(papers))
    for p in papers:
        log.debug("  Paper: %s (%s)", p.title, p.published)
    return papers


def _extract_papers(parsed: dict | list | None) -> List[Paper]:
    """Extract Paper objects from parsed JSON (tool output or search result)."""
    if isinstance(parsed, dict) and "papers" in parsed:
        paper_list = parsed["papers"]
    elif isinstance(parsed, list):
        paper_list = parsed
    else:
        return []

    papers: List[Paper] = []
    for p in paper_list:
        if isinstance(p, dict) and "title" in p:
            papers.append(
                Paper(
                    title=p["title"],
                    authors=p.get("authors", []),
                    summary=" ".join(p.get("summary", "").split()),
                    pdf_url=p.get("pdf_url", ""),
                    entry_id=p.get("id", ""),
                    published=p.get("published", ""),
                )
            )
    return papers




# -----------------------------
# Simple web search
# -----------------------------
# This uses DuckDuckGo HTML search results as a lightweight starter.
# For production, swap this with Tavily, SerpAPI, Exa, Perplexity Sonar, etc.

DUCKDUCKGO_HTML = "https://html.duckduckgo.com/html/"


async def search_web(query_text: str, max_results: int = 5) -> List[WebResult]:
    """
    Very lightweight HTML search.
    Replace with your preferred search API for production reliability.
    """
    log.info("[bold cyan]Web search[/]: %s", query_text)
    results: List[WebResult] = []

    async with httpx.AsyncClient(timeout=20.0, follow_redirects=True) as client:
        resp = await client.post(
            DUCKDUCKGO_HTML,
            data={"q": query_text},
            headers={
                "User-Agent": "Mozilla/5.0",
            },
        )
        resp.raise_for_status()

    soup = BeautifulSoup(resp.text, "html.parser")
    links = soup.select(".result")

    for item in links[:max_results]:
        title_el = item.select_one(".result__title")
        snippet_el = item.select_one(".result__snippet")
        link_el = item.select_one(".result__url, .result__a")

        title = title_el.get_text(" ", strip=True) if title_el else ""
        snippet = snippet_el.get_text(" ", strip=True) if snippet_el else ""
        url = ""

        a = item.select_one(".result__a")
        if a and a.has_attr("href"):
            url = a["href"]

        if title and url:
            results.append(WebResult(title=title, url=url, snippet=snippet))

    log.info("Web search returned [green]%d[/] results", len(results))
    return results


async def fetch_page_text(url: str, max_chars: int = 4000) -> str:
    """
    Fetch page text and truncate aggressively.
    """
    log.debug("Fetching page: %s", url)
    try:
        async with httpx.AsyncClient(timeout=20.0, follow_redirects=True) as client:
            resp = await client.get(
                url,
                headers={"User-Agent": "Mozilla/5.0"},
            )
            resp.raise_for_status()

        soup = BeautifulSoup(resp.text, "html.parser")

        # remove noisy tags
        for tag in soup(["script", "style", "noscript", "header", "footer", "svg"]):
            tag.decompose()

        text = soup.get_text("\n", strip=True)
        text = re.sub(r"\n{2,}", "\n\n", text)
        log.debug("Fetched %d chars from %s", min(len(text), max_chars), url)
        return text[:max_chars]
    except Exception as exc:
        log.warning("Failed to fetch %s: %s", url, exc)
        return f"[Failed to fetch content from {url}: {exc}]"


# -----------------------------
# Claude helpers
# -----------------------------

async def run_claude(prompt: str, system_prompt: str, max_turns: int = 4) -> tuple[str, dict]:
    """
    Run a Claude Agent SDK query and collect plain text output plus metadata.
    """
    options = ClaudeAgentOptions(
        system_prompt=system_prompt,
        max_turns=max_turns,
        # Keep tool permissions minimal here because we already gather context outside.
        # You can expand this later if you want Claude itself to write files, use bash, etc.
        allowed_tools=[],
        disallowed_tools=["Bash", "Write", "Edit"],
    )

    chunks: List[str] = []
    meta: dict = {}

    log.info("[bold magenta]Calling Claude[/] (max_turns=%d)", max_turns)
    async for message in query(prompt=prompt, options=options):
        if isinstance(message, AssistantMessage):
            for block in message.content:
                if isinstance(block, TextBlock):
                    chunks.append(block.text)
            if not meta.get("model") and hasattr(message, "model"):
                meta["model"] = message.model
        elif isinstance(message, ResultMessage):
            meta["usage"] = message.usage or {}
            meta["model_usage"] = message.model_usage or {}
            meta["total_cost_usd"] = message.total_cost_usd
            meta["duration_ms"] = message.duration_ms
            meta["duration_api_ms"] = message.duration_api_ms
            meta["num_turns"] = message.num_turns

    text = "\n".join(chunks).strip()
    log.info(
        "Claude responded: [green]%d[/] chars, %d turns, %s ms, $%s",
        len(text),
        meta.get("num_turns", 0),
        meta.get("duration_ms", "?"),
        f"{meta.get('total_cost_usd', 0):.4f}" if meta.get("total_cost_usd") else "?",
    )
    log.debug("Usage: %s", meta.get("usage"))
    return text, meta


# -----------------------------
# Prompt builders
# -----------------------------

def build_paper_context(papers: List[Paper]) -> str:
    paper_blocks = []
    for i, p in enumerate(papers, start=1):
        paper_blocks.append(
            textwrap.dedent(
                f"""
                [{i}] {p.title}
                Authors: {", ".join(p.authors)}
                Published: {p.published}
                arXiv: {p.entry_id}
                PDF: {p.pdf_url}
                Abstract:
                {p.summary}
                """
            ).strip()
        )
    return "\n\n".join(paper_blocks)


def build_web_context(results: List[WebResult]) -> str:
    web_blocks = []
    for i, r in enumerate(results, start=1):
        content = (r.content or "").strip()
        web_blocks.append(
            textwrap.dedent(
                f"""
                [{i}] {r.title}
                URL: {r.url}
                Snippet: {r.snippet}

                Extracted page text:
                {content[:3000]}
                """
            ).strip()
        )
    return "\n\n".join(web_blocks)


# -----------------------------
# Main workflow
# -----------------------------

async def research_and_summarize(user_input: str) -> dict:
    with langfuse.start_as_current_observation(
        name="research_and_summarize",
        as_type="span",
        input={"user_input": user_input},
    ) as trace:
        try:
            result = await _research_and_summarize_traced(user_input, trace)
            trace.update(output={
                "has_summary": bool(result["final_summary"]),
                "num_papers": len(result["papers"]),
                "num_web_results": len(result["web_results"]),
            })
            return result
        except Exception as e:
            trace.update(output={"error": str(e)}, level="ERROR")
            raise


async def _research_and_summarize_traced(user_input: str, trace) -> dict:
    log.info("[bold]Step 1/5:[/] arXiv retrieval (skill: extract query + search)")
    # 1) arXiv skill: Claude extracts search params from NL, calls the search tool
    arxiv_span = trace.start_observation(name="arxiv_skill", input={"user_input": user_input})
    papers = await run_arxiv_skill(user_input)
    arxiv_span.update(output={"num_papers": len(papers), "titles": [p.title for p in papers]})
    arxiv_span.end()
    paper_context = build_paper_context(papers)

    log.info("[bold]Step 2/5:[/] Claude critique pass")
    # 2) First Claude pass: critique papers + decide what else to search
    critique_system = (
        "You are a rigorous research analyst. "
        "Critique papers for novelty, methodology, evidence quality, limitations, "
        "hidden assumptions, evaluation weaknesses, and practical significance. "
        "Be skeptical but fair."
    )

    critique_prompt = f"""
User request:
{user_input}

Candidate arXiv papers:
{paper_context}

Tasks:
1. Identify the 3 most relevant papers.
2. Critique each one.
3. Point out agreement/disagreement across papers.
4. Suggest 5 targeted follow-on web searches that would help validate or contextualize the claims.
5. Return your answer in JSON with keys:
   - selected_papers
   - critiques
   - comparison
   - follow_on_searches
"""

    critique_gen = trace.start_observation(
        name="claude_critique",
        as_type="generation",
        input={"prompt": critique_prompt, "system_prompt": critique_system},
        metadata={"purpose": "critique_papers", "max_turns": 4},
    )
    critique_raw, critique_meta = await run_claude(critique_prompt, critique_system)
    critique_gen.update(
        output=critique_raw,
        model=critique_meta.get("model"),
        usage_details={
            "input": critique_meta.get("usage", {}).get("input_tokens", 0),
            "output": critique_meta.get("usage", {}).get("output_tokens", 0),
        },
        metadata={
            "total_cost_usd": critique_meta.get("total_cost_usd"),
            "duration_ms": critique_meta.get("duration_ms"),
            "duration_api_ms": critique_meta.get("duration_api_ms"),
            "num_turns": critique_meta.get("num_turns"),
            "model_usage": critique_meta.get("model_usage"),
        },
    )
    critique_gen.end()
    parsed = try_parse_json(critique_raw)

    raw_searches = parsed.get("follow_on_searches", []) if isinstance(parsed, dict) else []
    # Claude may return strings or dicts like {"query": "...", "rationale": "..."}
    follow_on_searches = []
    for s in raw_searches:
        if isinstance(s, str):
            follow_on_searches.append(s)
        elif isinstance(s, dict) and "query" in s:
            follow_on_searches.append(s["query"])
    if not follow_on_searches:
        follow_on_searches = [
            f"{user_input} benchmark results",
            f"{user_input} blog analysis",
            f"{user_input} github implementation",
        ]

    log.info("[bold]Step 3/5:[/] Web searches (%d queries)", len(follow_on_searches[:3]))
    # 3) Web search
    web_search_span = trace.start_observation(
        name="web_searches",
        input={"queries": follow_on_searches[:3]},
    )
    web_results: List[WebResult] = []
    for q in follow_on_searches[:3]:
        child = web_search_span.start_observation(name="search_web", input={"query": q})
        results = await search_web(q, max_results=2)
        child.update(output={"num_results": len(results)})
        child.end()
        web_results.extend(results)

    # dedupe by URL
    deduped = []
    seen = set()
    for r in web_results:
        if r.url not in seen:
            seen.add(r.url)
            deduped.append(r)
    web_results = deduped[:5]
    web_search_span.update(
        output={"num_results": len(web_results), "urls": [r.url for r in web_results]},
    )
    web_search_span.end()

    log.info("[bold]Step 4/5:[/] Fetching %d pages", len(web_results))
    # 4) Fetch page text
    fetch_span = trace.start_observation(
        name="fetch_pages",
        input={"urls": [r.url for r in web_results]},
    )
    enriched_results: List[WebResult] = []
    for r in web_results:
        child = fetch_span.start_observation(name="fetch_page_text", input={"url": r.url})
        content = await fetch_page_text(r.url, max_chars=4000)
        child.update(output={"content_length": len(content)})
        child.end()
        enriched_results.append(
            WebResult(
                title=r.title,
                url=r.url,
                snippet=r.snippet,
                content=content,
            )
        )
    fetch_span.update(output={"num_fetched": len(enriched_results)})
    fetch_span.end()

    web_context = build_web_context(enriched_results)

    log.info("[bold]Step 5/5:[/] Final synthesis")
    # 5) Final synthesis
    summary_system = (
        "You are an expert research synthesizer. "
        "Produce precise, useful summaries. "
        "Separate evidence from speculation."
    )

    final_prompt = f"""
User request:
{user_input}

arXiv paper context:
{paper_context}

Initial critique pass:
{critique_raw}

External web context:
{web_context}

Now produce:
1. A concise executive summary
2. A detailed synthesis
3. What appears well-supported
4. What remains uncertain
5. Practical takeaways
6. A final ranked reading list

Use clear section headers.
"""

    final_gen = trace.start_observation(
        name="claude_synthesis",
        as_type="generation",
        input={"prompt": final_prompt, "system_prompt": summary_system},
        metadata={"purpose": "final_synthesis", "max_turns": 4},
    )
    final_summary, final_meta = await run_claude(final_prompt, summary_system)
    final_gen.update(
        output=final_summary,
        model=final_meta.get("model"),
        usage_details={
            "input": final_meta.get("usage", {}).get("input_tokens", 0),
            "output": final_meta.get("usage", {}).get("output_tokens", 0),
        },
        metadata={
            "total_cost_usd": final_meta.get("total_cost_usd"),
            "duration_ms": final_meta.get("duration_ms"),
            "duration_api_ms": final_meta.get("duration_api_ms"),
            "num_turns": final_meta.get("num_turns"),
            "model_usage": final_meta.get("model_usage"),
        },
    )
    final_gen.end()

    log.info("[bold green]Research complete![/] %d papers, %d web results", len(papers), len(enriched_results))
    return {
        "user_input": user_input,
        "papers": [asdict(p) for p in papers],
        "initial_critique": parsed if parsed else critique_raw,
        "web_results": [asdict(r) for r in enriched_results],
        "final_summary": final_summary,
    }


def try_parse_json(text: str) -> dict | None:
    """
    Best-effort parse for model JSON output.
    """
    text = text.strip()

    # direct parse
    try:
        obj = json.loads(text)
        if isinstance(obj, dict):
            return obj
    except Exception:
        pass

    # fenced json parse
    match = re.search(r"```json\s*(\{.*?\})\s*```", text, flags=re.DOTALL)
    if match:
        try:
            obj = json.loads(match.group(1))
            if isinstance(obj, dict):
                return obj
        except Exception:
            pass

    return None


# -----------------------------
# Cross-report summarization
# -----------------------------

_CROSS_REPORT_PROMPT = (
    "You are a research meta-analyst with access to a memory store of past research reports. "
    "When asked to summarize across topics: "
    "(1) search for relevant reports using search_past_reports, "
    "(2) retrieve full reports using get_report for the most relevant matches, "
    "(3) identify themes, contradictions, and evolution across reports, "
    "(4) produce a cross-cutting synthesis citing specific reports and papers. "
    "Output the synthesis as clear markdown."
)


async def cross_report_summary(query_text: str) -> str:
    """Agent-driven cross-report summarization.

    Searches past reports, retrieves the relevant ones, and synthesizes
    themes and findings across them.
    """
    options = ClaudeAgentOptions(
        system_prompt=_CROSS_REPORT_PROMPT,
        max_turns=10,
        permission_mode="bypassPermissions",
        mcp_servers={"memory": memory_mcp_server},
    )

    text_chunks: List[str] = []
    try:
        async with ClaudeSDKClient(options=options) as client:
            await client.connect(query_text)
            async for msg in client.receive_messages():
                if isinstance(msg, AssistantMessage):
                    for block in msg.content:
                        if isinstance(block, TextBlock):
                            text_chunks.append(block.text)
                if isinstance(msg, ResultMessage):
                    break
    except Exception as exc:
        log.warning("Cross-report summary error: %s", exc)

    return "\n".join(text_chunks).strip() or "No cross-report summary could be generated."


async def main() -> None:
    user_input = (
        "how to manage agent memory in a longitudinal way?"
    )

    console.rule("[bold blue]arXiv Deep Research[/]")
    log.info("Starting research: %s", user_input)

    result = await research_and_summarize(user_input)

    # Persist to memory
    report_id = store_report_to_db(result)
    log.info("Report stored in memory: [cyan]%s[/]", report_id)

    console.rule("[bold green]Final Summary[/]")
    console.print(result["final_summary"])

    console.rule("[bold green]Papers[/]")
    for p in result["papers"]:
        console.print(f"  - {p['title']} ({p['published']})")

    console.rule("[bold green]Web Results[/]")
    for r in result["web_results"]:
        console.print(f"  - {r['title']}: {r['url']}")

    langfuse.flush()
    log.info("Done. Logs saved to [cyan]logs/research.log[/]")


if __name__ == "__main__":
    anyio.run(main)