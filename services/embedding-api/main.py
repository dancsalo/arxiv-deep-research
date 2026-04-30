from fastapi import FastAPI
from pydantic import BaseModel, field_validator
from sentence_transformers import SentenceTransformer

app = FastAPI()

MAX_TEXTS = 100
MAX_CHARS_PER_TEXT = 10_000


class EmbedRequest(BaseModel):
    texts: list[str]

    @field_validator("texts")
    @classmethod
    def validate_texts(cls, v):
        if len(v) == 0:
            raise ValueError("texts must not be empty")
        if len(v) > MAX_TEXTS:
            raise ValueError(f"max {MAX_TEXTS} texts per request, got {len(v)}")
        return [t[:MAX_CHARS_PER_TEXT] for t in v]


class EmbedResponse(BaseModel):
    embeddings: list[list[float]]
    dimensions: int


model = SentenceTransformer("all-MiniLM-L6-v2")


@app.post("/embed", response_model=EmbedResponse)
def embed(req: EmbedRequest):
    vectors = model.encode(req.texts, batch_size=32, normalize_embeddings=True)
    return EmbedResponse(
        embeddings=vectors.tolist(),
        dimensions=vectors.shape[1],
    )


@app.get("/health")
def health():
    return {"status": "ok", "model": "all-MiniLM-L6-v2", "dimensions": 384}
