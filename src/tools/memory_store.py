"""SQLite-backed memory store for research reports."""

from __future__ import annotations

import json
import sqlite3
import uuid
from datetime import datetime, timezone
from pathlib import Path
from typing import Optional

DB_PATH = Path(__file__).resolve().parents[2] / "data" / "research_memory.db"

_SCHEMA_VERSION = 1

_CREATE_TABLES = """
CREATE TABLE IF NOT EXISTS reports (
    id            TEXT PRIMARY KEY,
    topic         TEXT NOT NULL,
    critique      TEXT,
    final_summary TEXT,
    created_at    TEXT NOT NULL,
    updated_at    TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS papers (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    report_id   TEXT NOT NULL REFERENCES reports(id),
    title       TEXT NOT NULL,
    authors     TEXT,
    summary     TEXT,
    pdf_url     TEXT,
    entry_id    TEXT,
    published   TEXT
);
CREATE INDEX IF NOT EXISTS idx_papers_report ON papers(report_id);
CREATE INDEX IF NOT EXISTS idx_papers_entry  ON papers(entry_id);

CREATE TABLE IF NOT EXISTS web_results (
    id        INTEGER PRIMARY KEY AUTOINCREMENT,
    report_id TEXT NOT NULL REFERENCES reports(id),
    title     TEXT,
    url       TEXT,
    snippet   TEXT,
    content   TEXT
);
CREATE INDEX IF NOT EXISTS idx_web_report ON web_results(report_id);
"""


def _get_conn() -> sqlite3.Connection:
    """Get a connection, creating DB + tables on first call."""
    DB_PATH.parent.mkdir(parents=True, exist_ok=True)
    conn = sqlite3.connect(str(DB_PATH))
    conn.row_factory = sqlite3.Row
    conn.execute("PRAGMA journal_mode=WAL")
    conn.execute("PRAGMA foreign_keys=ON")
    conn.executescript(_CREATE_TABLES)
    return conn


def _now_iso() -> str:
    return datetime.now(timezone.utc).isoformat()


def _serialize_critique(critique) -> str:
    """Serialize critique — could be a dict or a string."""
    if isinstance(critique, dict):
        return json.dumps(critique)
    return str(critique) if critique else ""


def _deserialize_critique(raw: str):
    """Attempt to parse critique as JSON, fall back to raw string."""
    if not raw:
        return raw
    try:
        return json.loads(raw)
    except (json.JSONDecodeError, TypeError):
        return raw


# ---------------------
# Public API
# ---------------------


def store_report(report: dict) -> str:
    """Store a complete research report. Returns the report UUID.

    Accepts the exact dict shape from research_and_summarize:
      {user_input, papers, initial_critique, web_results, final_summary}
    """
    conn = _get_conn()
    report_id = str(uuid.uuid4())
    now = _now_iso()

    try:
        conn.execute(
            "INSERT INTO reports (id, topic, critique, final_summary, created_at, updated_at) "
            "VALUES (?, ?, ?, ?, ?, ?)",
            (
                report_id,
                report.get("user_input", ""),
                _serialize_critique(report.get("initial_critique")),
                report.get("final_summary", ""),
                now,
                now,
            ),
        )

        papers = report.get("papers", [])
        for p in papers:
            authors = p.get("authors", [])
            conn.execute(
                "INSERT INTO papers (report_id, title, authors, summary, pdf_url, entry_id, published) "
                "VALUES (?, ?, ?, ?, ?, ?, ?)",
                (
                    report_id,
                    p.get("title", ""),
                    json.dumps(authors) if isinstance(authors, list) else str(authors),
                    p.get("summary", ""),
                    p.get("pdf_url", ""),
                    p.get("entry_id", p.get("id", "")),
                    p.get("published", ""),
                ),
            )

        web_results = report.get("web_results", [])
        for w in web_results:
            conn.execute(
                "INSERT INTO web_results (report_id, title, url, snippet, content) "
                "VALUES (?, ?, ?, ?, ?)",
                (
                    report_id,
                    w.get("title", ""),
                    w.get("url", ""),
                    w.get("snippet", ""),
                    w.get("content", ""),
                ),
            )

        conn.commit()
    finally:
        conn.close()

    return report_id


def search_reports(
    keyword: Optional[str] = None,
    date_from: Optional[str] = None,
    date_to: Optional[str] = None,
    paper_entry_id: Optional[str] = None,
    limit: int = 20,
) -> list[dict]:
    """Search reports. Returns lightweight summaries."""
    conn = _get_conn()
    try:
        conditions = []
        params: list = []

        if keyword:
            conditions.append("(r.topic LIKE ? OR r.final_summary LIKE ?)")
            params.extend([f"%{keyword}%", f"%{keyword}%"])

        if date_from:
            conditions.append("r.created_at >= ?")
            params.append(date_from)

        if date_to:
            conditions.append("r.created_at <= ?")
            params.append(date_to)

        join_papers = ""
        if paper_entry_id:
            join_papers = "JOIN papers p ON p.report_id = r.id"
            conditions.append("p.entry_id = ?")
            params.append(paper_entry_id)

        where = f"WHERE {' AND '.join(conditions)}" if conditions else ""

        sql = f"""
            SELECT DISTINCT r.id, r.topic, r.created_at, r.updated_at,
                   substr(r.final_summary, 1, 300) as summary_snippet,
                   (SELECT COUNT(*) FROM papers WHERE report_id = r.id) as num_papers,
                   (SELECT COUNT(*) FROM web_results WHERE report_id = r.id) as num_web_results
            FROM reports r
            {join_papers}
            {where}
            ORDER BY r.created_at DESC
            LIMIT ?
        """
        params.append(limit)

        rows = conn.execute(sql, params).fetchall()
        return [dict(row) for row in rows]
    finally:
        conn.close()


def get_report(report_id: str) -> Optional[dict]:
    """Retrieve a full report by ID, including papers and web_results."""
    conn = _get_conn()
    try:
        row = conn.execute("SELECT * FROM reports WHERE id = ?", (report_id,)).fetchone()
        if not row:
            return None

        report = dict(row)
        report["critique"] = _deserialize_critique(report.get("critique", ""))

        paper_rows = conn.execute(
            "SELECT title, authors, summary, pdf_url, entry_id, published "
            "FROM papers WHERE report_id = ?",
            (report_id,),
        ).fetchall()
        report["papers"] = []
        for p in paper_rows:
            pd = dict(p)
            try:
                pd["authors"] = json.loads(pd["authors"])
            except (json.JSONDecodeError, TypeError):
                pd["authors"] = []
            report["papers"].append(pd)

        web_rows = conn.execute(
            "SELECT title, url, snippet, content FROM web_results WHERE report_id = ?",
            (report_id,),
        ).fetchall()
        report["web_results"] = [dict(w) for w in web_rows]

        return report
    finally:
        conn.close()


def append_to_report(
    report_id: str,
    new_papers: Optional[list[dict]] = None,
    new_web_results: Optional[list[dict]] = None,
    revised_summary: Optional[str] = None,
    additional_critique: Optional[str] = None,
) -> bool:
    """Append new findings to an existing report. Returns True on success."""
    conn = _get_conn()
    try:
        row = conn.execute("SELECT id FROM reports WHERE id = ?", (report_id,)).fetchone()
        if not row:
            return False

        now = _now_iso()

        if new_papers:
            for p in new_papers:
                authors = p.get("authors", [])
                conn.execute(
                    "INSERT INTO papers (report_id, title, authors, summary, pdf_url, entry_id, published) "
                    "VALUES (?, ?, ?, ?, ?, ?, ?)",
                    (
                        report_id,
                        p.get("title", ""),
                        json.dumps(authors) if isinstance(authors, list) else str(authors),
                        p.get("summary", ""),
                        p.get("pdf_url", ""),
                        p.get("entry_id", p.get("id", "")),
                        p.get("published", ""),
                    ),
                )

        if new_web_results:
            for w in new_web_results:
                conn.execute(
                    "INSERT INTO web_results (report_id, title, url, snippet, content) "
                    "VALUES (?, ?, ?, ?, ?)",
                    (
                        report_id,
                        w.get("title", ""),
                        w.get("url", ""),
                        w.get("snippet", ""),
                        w.get("content", ""),
                    ),
                )

        if revised_summary:
            conn.execute(
                "UPDATE reports SET final_summary = ?, updated_at = ? WHERE id = ?",
                (revised_summary, now, report_id),
            )

        if additional_critique:
            existing = conn.execute(
                "SELECT critique FROM reports WHERE id = ?", (report_id,)
            ).fetchone()
            old = existing["critique"] or ""
            combined = f"{old}\n\n--- Additional critique ({now}) ---\n{additional_critique}" if old else additional_critique
            conn.execute(
                "UPDATE reports SET critique = ?, updated_at = ? WHERE id = ?",
                (combined, now, report_id),
            )

        conn.execute(
            "UPDATE reports SET updated_at = ? WHERE id = ?", (now, report_id)
        )
        conn.commit()
        return True
    finally:
        conn.close()


def get_reports_by_ids(report_ids: list[str]) -> list[dict]:
    """Retrieve multiple full reports for cross-report work."""
    results = []
    for rid in report_ids:
        report = get_report(rid)
        if report:
            results.append(report)
    return results
