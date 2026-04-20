from typing import Literal, Optional
from urllib.parse import urlencode
import requests
import xml.etree.ElementTree as ET
from pydantic import BaseModel, Field

ARXIV_API_URL = "https://export.arxiv.org/api/query"

class SearchArxivInput(BaseModel):
    query: str = Field(description="Plain-English paper search query")
    categories: list[str] = Field(
        default_factory=list,
        description="Optional arXiv categories like cs.LG or cs.AI"
    )
    max_results: int = Field(default=5, ge=1, le=25)
    start: int = Field(default=0, ge=0)
    sort_by: Literal["relevance", "lastUpdatedDate", "submittedDate"] = "relevance"
    sort_order: Literal["ascending", "descending"] = "descending"

class SearchArxivOutput(BaseModel):
    status: Literal["success", "error"]
    total_results: Optional[int] = None
    papers: list[dict] = Field(default_factory=list)
    error_message: Optional[str] = None

def build_search_query(query: str, categories: list[str]) -> str:
    search_expr = f"all:{query}"
    if categories:
        cat_expr = " OR ".join(f"cat:{c}" for c in categories)
        search_expr = f"({search_expr}) AND ({cat_expr})"
    return search_expr

def parse_arxiv_feed(xml_text: str) -> tuple[int, list[dict]]:
    ns = {
        "atom": "http://www.w3.org/2005/Atom",
        "opensearch": "http://a9.com/-/spec/opensearch/1.1/",
        "arxiv": "http://arxiv.org/schemas/atom",
    }

    root = ET.fromstring(xml_text)

    total_results_el = root.find("opensearch:totalResults", ns)
    total_results = int(total_results_el.text) if total_results_el is not None else 0

    papers = []
    for entry in root.findall("atom:entry", ns):
        entry_id = entry.findtext("atom:id", default="", namespaces=ns)
        title = entry.findtext("atom:title", default="", namespaces=ns).strip()
        summary = entry.findtext("atom:summary", default="", namespaces=ns).strip()
        published = entry.findtext("atom:published", default="", namespaces=ns)
        updated = entry.findtext("atom:updated", default="", namespaces=ns)

        authors = [
            author.findtext("atom:name", default="", namespaces=ns)
            for author in entry.findall("atom:author", ns)
        ]

        categories = [
            cat.attrib.get("term", "")
            for cat in entry.findall("atom:category", ns)
        ]

        primary_category_el = entry.find("arxiv:primary_category", ns)
        primary_category = (
            primary_category_el.attrib.get("term")
            if primary_category_el is not None else None
        )

        abs_url = None
        pdf_url = None
        for link in entry.findall("atom:link", ns):
            href = link.attrib.get("href")
            rel = link.attrib.get("rel")
            title_attr = link.attrib.get("title")

            if rel == "alternate":
                abs_url = href
            elif title_attr == "pdf":
                pdf_url = href

        papers.append({
            "id": entry_id.split("/abs/")[-1] if "/abs/" in entry_id else entry_id,
            "title": title,
            "authors": authors,
            "summary": summary,
            "published": published,
            "updated": updated,
            "primary_category": primary_category,
            "categories": categories,
            "abs_url": abs_url,
            "pdf_url": pdf_url,
        })

    return total_results, papers

def search_arxiv_tool(input: SearchArxivInput) -> SearchArxivOutput:
    try:
        params = {
            "search_query": build_search_query(input.query, input.categories),
            "start": input.start,
            "max_results": input.max_results,
            "sortBy": input.sort_by,
            "sortOrder": input.sort_order,
        }

        response = requests.get(
            ARXIV_API_URL,
            params=params,
            timeout=20,
            headers={
                "User-Agent": "your-app-name/0.1 (contact: you@example.com)"
            },
        )
        response.raise_for_status()

        total_results, papers = parse_arxiv_feed(response.text)
        return SearchArxivOutput(
            status="success",
            total_results=total_results,
            papers=papers,
        )
    except Exception as e:
        return SearchArxivOutput(
            status="error",
            error_message=str(e),
        )
