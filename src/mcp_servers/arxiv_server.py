#!/usr/bin/env python3
"""arXiv search MCP server (stdio transport)."""

import sys
from pathlib import Path

# Ensure project root is importable regardless of how uv launches us
sys.path.insert(0, str(Path(__file__).resolve().parents[2]))

from mcp.server.fastmcp import FastMCP
from src.tools.arxiv_search import SearchArxivInput, search_arxiv_tool

server = FastMCP("arxiv-search")


@server.tool()
def search_arxiv(
    query: str,
    categories: list[str] | None = None,
    max_results: int = 10,
    sort_by: str = "relevance",
    sort_order: str = "descending",
    start: int = 0,
) -> str:
    """Search arXiv for academic papers.

    Args:
        query: Keyword query for arXiv (e.g. 'test-time scaling reasoning models').
        categories: Optional arXiv categories like cs.LG, cs.AI, cs.CL.
        max_results: Number of papers to return (1-25, default 10).
        sort_by: One of 'relevance', 'lastUpdatedDate', 'submittedDate'.
        sort_order: 'ascending' or 'descending'.
        start: Pagination offset.
    """
    search_input = SearchArxivInput(
        query=query,
        categories=categories or [],
        max_results=max_results,
        sort_by=sort_by,
        sort_order=sort_order,
        start=start,
    )
    result = search_arxiv_tool(search_input)
    return result.model_dump_json()


if __name__ == "__main__":
    server.run()
