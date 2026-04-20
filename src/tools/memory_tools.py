"""In-process SDK tool definitions for the research memory store."""

from __future__ import annotations

import json

from claude_agent_sdk import create_sdk_mcp_server, tool

from src.tools.memory_store import (
    append_to_report,
    get_report,
    search_reports,
    store_report,
)


@tool(
    "store_research_report",
    "Store a completed research report with papers, critique, web results, and summary.",
    {"topic": str, "papers": list, "critique": str, "web_results": list, "final_summary": str},
)
async def store_report_tool(args):
    report_dict = {
        "user_input": args["topic"],
        "papers": args.get("papers", []),
        "initial_critique": args.get("critique", ""),
        "web_results": args.get("web_results", []),
        "final_summary": args.get("final_summary", ""),
    }
    report_id = store_report(report_dict)
    return {"content": [{"type": "text", "text": json.dumps({"report_id": report_id, "status": "stored"})}]}


@tool(
    "search_past_reports",
    "Search past research reports by keyword, date range, or arXiv paper ID. All fields are optional.",
    {"keyword": str, "date_from": str, "date_to": str, "paper_entry_id": str, "limit": int},
)
async def search_reports_tool(args):
    results = search_reports(
        keyword=args.get("keyword"),
        date_from=args.get("date_from"),
        date_to=args.get("date_to"),
        paper_entry_id=args.get("paper_entry_id"),
        limit=args.get("limit", 20),
    )
    return {"content": [{"type": "text", "text": json.dumps(results)}]}


@tool(
    "get_report",
    "Retrieve a full research report by its ID, including all papers and web results.",
    {"report_id": str},
)
async def get_report_tool(args):
    report = get_report(args["report_id"])
    if report is None:
        return {"content": [{"type": "text", "text": json.dumps({"error": "Report not found"})}]}
    return {"content": [{"type": "text", "text": json.dumps(report)}]}


@tool(
    "append_to_report",
    "Add new papers, web results, or a revised summary to an existing report. All fields except report_id are optional.",
    {"report_id": str, "new_papers": list, "new_web_results": list, "revised_summary": str, "additional_critique": str},
)
async def append_to_report_tool(args):
    success = append_to_report(
        report_id=args["report_id"],
        new_papers=args.get("new_papers"),
        new_web_results=args.get("new_web_results"),
        revised_summary=args.get("revised_summary"),
        additional_critique=args.get("additional_critique"),
    )
    status = "updated" if success else "report_not_found"
    return {"content": [{"type": "text", "text": json.dumps({"status": status})}]}


memory_mcp_server = create_sdk_mcp_server(
    name="memory",
    tools=[store_report_tool, search_reports_tool, get_report_tool, append_to_report_tool],
)
