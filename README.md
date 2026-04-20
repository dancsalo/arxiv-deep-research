# arxiv-deep-research

A deep research tool that searches arXiv for academic papers, critiques them using Claude, performs follow-on web searches, and produces a comprehensive research synthesis.

## Setup

```bash
pip install -e .
```

## Usage

```bash
python example.py
```

## Langfuse Observability

The example script logs traces to [Langfuse](https://langfuse.com) for observability into each pipeline step: arXiv search, Claude critique, web searches, page fetches, and final synthesis.

### Run Langfuse locally with Docker

```bash
docker compose -f docker-compose.langfuse.yml up -d
```

Open http://localhost:3000 in your browser. On first launch:

1. Create an account (local only, no email verification needed)
2. Create a new project
3. Go to **Settings > API Keys** and copy the **Secret Key** and **Public Key**

### Configure environment variables

```bash
export LANGFUSE_SECRET_KEY="sk-lf-..."
export LANGFUSE_PUBLIC_KEY="pk-lf-..."
export LANGFUSE_HOST="http://localhost:3000"
```

### Run the example

```bash
python example.py
```

After the script completes, open the Langfuse UI at http://localhost:3000 and navigate to **Traces** to see the full pipeline trace with:

- **Generations** for each Claude call (critique + synthesis) with token usage and cost
- **Spans** for arXiv search, web searches, and page fetches

### Tear down

```bash
docker compose -f docker-compose.langfuse.yml down     # stop containers, keep data
docker compose -f docker-compose.langfuse.yml down -v   # stop containers and delete data
```
