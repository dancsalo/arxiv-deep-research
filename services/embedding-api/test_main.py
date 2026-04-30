import math

import pytest
from fastapi.testclient import TestClient

from main import app

client = TestClient(app)


def test_single_text_embedding():
    resp = client.post("/embed", json={"texts": ["hello"]})
    assert resp.status_code == 200
    data = resp.json()
    assert len(data["embeddings"]) == 1
    assert len(data["embeddings"][0]) == 384
    assert data["dimensions"] == 384


def test_batch_of_10():
    texts = [f"text {i}" for i in range(10)]
    resp = client.post("/embed", json={"texts": texts})
    assert resp.status_code == 200
    data = resp.json()
    assert len(data["embeddings"]) == 10
    for vec in data["embeddings"]:
        assert len(vec) == 384


def test_normalized_embeddings():
    resp = client.post("/embed", json={"texts": ["hello"]})
    vec = resp.json()["embeddings"][0]
    norm = math.sqrt(sum(x * x for x in vec))
    assert abs(norm - 1.0) < 0.01


def test_empty_texts_rejected():
    resp = client.post("/embed", json={"texts": []})
    assert resp.status_code == 422
    assert "must not be empty" in resp.text


def test_over_100_texts_rejected():
    resp = client.post("/embed", json={"texts": ["a"] * 101})
    assert resp.status_code == 422
    assert "max 100" in resp.text


def test_exactly_100_texts_accepted():
    resp = client.post("/embed", json={"texts": ["a"] * 100})
    assert resp.status_code == 200
    assert len(resp.json()["embeddings"]) == 100


def test_long_text_truncated_not_rejected():
    resp = client.post("/embed", json={"texts": ["x" * 20000]})
    assert resp.status_code == 200
    assert len(resp.json()["embeddings"]) == 1


def test_health_endpoint():
    resp = client.get("/health")
    assert resp.status_code == 200
    data = resp.json()
    assert data["status"] == "ok"
    assert data["model"] == "all-MiniLM-L6-v2"
    assert data["dimensions"] == 384


def test_missing_body():
    resp = client.post("/embed")
    assert resp.status_code == 422


def test_same_text_deterministic():
    resp1 = client.post("/embed", json={"texts": ["hello"]})
    resp2 = client.post("/embed", json={"texts": ["hello"]})
    assert resp1.json()["embeddings"] == resp2.json()["embeddings"]
