# Python

Use this skill for Python application code, APIs, scripts, automation, data/ML work, AI integrations, notebooks promoted into production, 2D/3D game prototypes, CLI tools, and backend services. Keep guidance practical, stack-aware, and minimal-diff. Do not use this skill for non-Python repos just because a tool invokes Python internally.

---

## Routing Rules

Use when the repo or task includes any of:

- `.py`, `.pyi`, `.ipynb`, `pyproject.toml`, `requirements.txt`, `setup.py`, `setup.cfg`, `Pipfile`, `poetry.lock`, `uv.lock`, `conda.yaml`, `environment.yml`
- FastAPI, Flask, Django, Starlette, Celery, RQ, SQLAlchemy, Alembic, Pydantic
- NumPy, pandas, scikit-learn, PyTorch, TensorFlow, JAX, Hugging Face, sentence-transformers, OpenAI/Anthropic SDKs, LangChain/LlamaIndex, MLflow, wandb
- pygame, arcade, pyglet, Panda3D, Ursina, ModernGL, Blender Python scripts/addons

Do not use for pure frontend tasks, shell-only deployment work, or language-agnostic code review unless Python files are touched.

---

## Operating Standard

Before editing, infer the Python project shape from files, not assumptions:

- Runtime: Python version from `.python-version`, `pyproject.toml`, Dockerfile, CI, or existing syntax.
- Package manager: respect `uv.lock`, `poetry.lock`, `requirements.txt`, `Pipfile.lock`, `conda.yaml`, or existing scripts.
- App entrypoints: inspect `main.py`, `app.py`, `manage.py`, `src/`, package modules, service files, Docker/Celery commands, and tests.
- Style: mirror existing formatting, import style, logging, exceptions, typing strictness, and framework conventions.

Prefer small, behavior-preserving patches. Do not convert the dependency manager, project layout, web framework, or ML framework unless explicitly requested.

---

## 80/20 Rules

### 1. Respect environment and dependency discipline

Do not blindly add packages. Check whether the repo already uses an equivalent dependency.

Common commands, chosen by project files:

```bash
# uv
uv sync
uv add <package>
uv run pytest

# poetry
poetry install
poetry add <package>
poetry run pytest

# pip/venv
python -m venv .venv
python -m pip install -r requirements.txt
python -m pytest
```

Use `python -m <tool>` when path ambiguity is possible:

```bash
python -m pytest
python -m pip install -r requirements.txt
python -m ruff check .
```

Avoid committing generated caches: `__pycache__`, `.pytest_cache`, `.mypy_cache`, model artifacts, large datasets, local `.env` files.

### 2. Make boundaries typed and validated

Use type hints where they clarify contracts. Do not over-type tiny local variables.

```python
from dataclasses import dataclass

@dataclass(frozen=True)
class DetectionResult:
    score: float
    label: str
    reasons: list[str]
```

For API/config/external data, prefer Pydantic if already present:

```python
from pydantic import BaseModel, Field

class CreateJobRequest(BaseModel):
    project_id: str = Field(min_length=1)
    priority: int = Field(ge=0, le=10)
```

Avoid returning loose dicts from core logic when a dataclass, Pydantic model, `TypedDict`, or small result object would prevent shape drift.

### 3. Config, secrets, and paths must be explicit

Never hardcode secrets, tokens, production URLs, or user-specific absolute paths.

```python
import os


def required_env(name: str) -> str:
    value = os.getenv(name)
    if not value:
        raise RuntimeError(f"Missing required environment variable: {name}")
    return value

DATABASE_URL = required_env("DATABASE_URL")
```

Use `pathlib` for paths and anchor relative paths to a known base, not the current shell directory.

```python
from pathlib import Path

BASE_DIR = Path(__file__).resolve().parent
MODEL_PATH = BASE_DIR / "models" / "classifier.pt"
```

### 4. API code: validate input, control errors, avoid blocking the server

FastAPI high-frequency pattern:

```python
from fastapi import FastAPI, HTTPException
from pydantic import BaseModel, Field

app = FastAPI()

class PredictRequest(BaseModel):
    features: list[float] = Field(min_length=1)

class PredictResponse(BaseModel):
    score: float
    label: str

@app.post("/predict", response_model=PredictResponse)
def predict(payload: PredictRequest) -> PredictResponse:
    try:
        score = run_model(payload.features)
    except ValueError as exc:
        raise HTTPException(status_code=400, detail=str(exc)) from exc
    return PredictResponse(score=score, label="cheat" if score >= 0.5 else "legit")
```

API rules:

- Load heavy models/clients once at startup or lazy-cache them, not per request.
- Do not leak tracebacks or internal exceptions to clients.
- Use correct status codes; failed operations are not `200 OK`.
- Validate request bodies, query params, file names, and uploaded content.
- For `async def`, do not call blocking CPU/GPU/file/network work directly unless it is fast or already async-aware.
- Keep CORS narrow. Do not use `allow_origins=["*"]` with credentials.

### 5. Async/concurrency: separate I/O from CPU work

Use async for concurrent I/O. Use workers/processes/queues for heavy CPU or GPU jobs.

```python
import asyncio
import httpx

async def fetch_many(urls: list[str]) -> list[str]:
    async with httpx.AsyncClient(timeout=10) as client:
        responses = await asyncio.gather(*(client.get(url) for url in urls))
    return [response.text for response in responses]
```

Avoid:

```python
# Bad inside async request handlers: blocks the event loop
import requests
requests.get(url)
```

For background jobs, prefer the existing stack: Celery, RQ, Dramatiq, FastAPI `BackgroundTasks`, cron/systemd timers, or a queue already in the repo.

### 6. AI/ML Python: make inference reproducible, bounded, and batch-aware

Common libraries and where they fit:

- `numpy`: numerical arrays, vectorized math, tensor preprocessing.
- `pandas`: tabular data, CSV/parquet workflows, analysis; avoid in hot request loops unless necessary.
- `scikit-learn`: classical ML, preprocessing, metrics, calibration, model selection.
- `torch` / `tensorflow` / `jax`: deep learning, GPU inference/training.
- `transformers`, `sentence-transformers`: embeddings, LLM/NLP/CV model loading.
- `pydantic`: request/response schemas and config validation.
- `mlflow`, `wandb`, `tensorboard`: experiment tracking if already used.

Inference checklist:

- Load model once; call `eval()` for PyTorch inference.
- Use `torch.no_grad()` or `torch.inference_mode()`.
- Place tensors/model on the intended device consistently.
- Batch requests when throughput matters; cap batch size and payload size.
- Validate feature order, shape, dtype, normalization, and missing values.
- Keep training-only code out of request handlers.
- Record model version, threshold, and preprocessing version with predictions.

```python
import torch

DEVICE = torch.device("cuda" if torch.cuda.is_available() else "cpu")
model = load_model().to(DEVICE)
model.eval()


def predict_batch(features: torch.Tensor) -> torch.Tensor:
    features = features.to(DEVICE)
    with torch.inference_mode():
        logits = model(features)
        return torch.sigmoid(logits).detach().cpu()
```

For LLM/agent code:

- Treat model output as untrusted text/data; parse and validate before acting.
- Add timeouts/retries/backoff around API calls.
- Never put secrets, private keys, or raw sensitive data into prompts unless explicitly required and approved.
- Keep tool permissions narrow; log actions, not secrets.

### 7. Data scripts: make them rerunnable

Data/ETL scripts should be deterministic and safe to rerun.

```python
from pathlib import Path
import pandas as pd


def build_report(input_path: Path, output_path: Path) -> None:
    df = pd.read_csv(input_path)
    df = df.dropna(subset=["user_id"])
    output_path.parent.mkdir(parents=True, exist_ok=True)
    df.to_parquet(output_path, index=False)
```

Rules:

- Prefer vectorized pandas/NumPy operations over row-by-row loops for large data.
- Be explicit about encodings, time zones, dtypes, and missing values.
- Do not overwrite raw data unless the task explicitly asks.
- For large files, stream/chunk instead of loading everything into memory.

### 8. Automation and CLI: idempotent, observable, safe

Use `argparse`, `click`, or `typer` according to the repo. For simple scripts, standard `argparse` is enough.

```python
import argparse
from pathlib import Path


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("input", type=Path)
    parser.add_argument("--dry-run", action="store_true")
    args = parser.parse_args()

    if not args.input.exists():
        raise SystemExit(f"Input not found: {args.input}")

if __name__ == "__main__":
    main()
```

Use logging instead of scattered prints in long-running apps/services.

```python
import logging

logger = logging.getLogger(__name__)
logger.info("job_started", extra={"job_id": job_id})
```

For subprocesses, prefer `shell=False` and explicit args.

```python
import subprocess

subprocess.run(["git", "status", "--short"], check=True, text=True)
```

### 9. 2D games: stable loop, assets loaded once, delta time

Common stacks: `pygame`, `arcade`, `pyglet`. For Python 2D games, the main risks are bad loop timing, per-frame allocations, and asset reloads.

```python
import pygame

clock = pygame.time.Clock()
running = True

while running:
    dt = clock.tick(60) / 1000.0

    for event in pygame.event.get():
        if event.type == pygame.QUIT:
            running = False

    update(dt)
    draw()
    pygame.display.flip()
```

Rules:

- Load images/sounds/fonts once during startup or scene load.
- Use `dt` for movement; avoid frame-rate-dependent speeds.
- Keep collision logic deterministic and separated from rendering.
- Avoid disk I/O, network calls, or heavy object creation every frame.
- Use sprite groups/spatial partitioning when entity counts grow.

### 10. 3D games/tools: scene discipline and performance first

Common stacks: Panda3D, Ursina, pyglet/ModernGL, Blender Python. Python 3D is viable for tools/prototypes, but performance discipline matters.

Rules:

- Keep transforms explicit: position, rotation, scale, parent space vs world space.
- Load/cache meshes, textures, shaders, and materials once.
- Avoid per-frame mesh rebuilds unless necessary.
- Use fixed timestep or engine physics callbacks for physics-like behavior.
- Keep gameplay state separate from rendering nodes where possible.
- For Blender scripts/addons, respect context, object modes, and avoid destructive scene edits without clear user intent.

Minimal fixed-step pattern:

```python
FIXED_DT = 1.0 / 60.0
accumulator = 0.0


def frame(dt: float) -> None:
    global accumulator
    accumulator += min(dt, 0.25)
    while accumulator >= FIXED_DT:
        physics_update(FIXED_DT)
        accumulator -= FIXED_DT
    render_interpolated(accumulator / FIXED_DT)
```

### 11. Security-sensitive Python defaults

High-risk areas:

- `pickle`, `marshal`, `eval`, `exec`, unsafe YAML loaders.
- File uploads and archive extraction.
- Path traversal with user-controlled filenames.
- Shell commands with user input.
- SSRF through user-provided URLs.
- SQL string interpolation.
- Logging tokens, cookies, auth headers, prompts with secrets, or raw request bodies.

Safer YAML:

```python
import yaml

data = yaml.safe_load(text)
```

Safer path join:

```python
from pathlib import Path

ROOT = Path("/srv/uploads").resolve()
path = (ROOT / user_filename).resolve()
if not path.is_relative_to(ROOT):
    raise ValueError("Invalid path")
```

Do not unpickle untrusted files. Treat model files, datasets, user uploads, notebooks, and plugin scripts as executable-risk artifacts when they come from outside the project.

### 12. Verification: run the narrowest meaningful checks

Common checks:

```bash
python -m pytest
python -m pytest tests/test_specific.py -q
python -m ruff check .
python -m ruff format .
python -m mypy .
python -m pyright
python -m coverage run -m pytest
```

If checks fail, classify the failure:

- caused by the change
- pre-existing
- missing dependency/service/secret
- platform-specific
- test expects old behavior

Do not claim tests passed unless they were run successfully.

---

## Common AI Mistakes To Avoid

- Rewriting a working Python package layout because it looks unfamiliar.
- Adding new dependency managers or lockfiles to an existing project.
- Hiding errors with broad `except Exception: pass`.
- Using mutable defaults in functions/classes.
- Loading ML models inside every API request.
- Calling blocking `requests`, CPU-heavy pandas, or model inference directly inside hot async handlers.
- Forgetting `model.eval()` / `torch.inference_mode()` for PyTorch inference.
- Changing feature order, normalization, thresholds, or label mapping without updating the model contract.
- Treating notebooks as production modules without extracting stable functions.
- Using `pickle`, `eval`, `exec`, or `yaml.load` on untrusted input.
- Building shell commands by string concatenation.
- Hardcoding absolute local paths like `C:\Users\...` or `/home/user/...`.
- Returning raw stack traces from APIs.
- Creating per-frame allocations/disk loads in game loops.
- Making CLI scripts destructive without `--dry-run`, confirmation, backups, or clear output.
- Claiming a performance fix without measuring or at least identifying the hot path.

---

## High-Value Snippets

### Safe mutable defaults

```python
from dataclasses import dataclass, field

@dataclass
class Job:
    id: str
    events: list[str] = field(default_factory=list)
```

### Result object for expected failures

```python
from dataclasses import dataclass
from typing import Generic, TypeVar

T = TypeVar("T")

@dataclass(frozen=True)
class Result(Generic[T]):
    ok: bool
    value: T | None = None
    error: str | None = None
```

### Cached heavy model/client

```python
from functools import lru_cache

@lru_cache(maxsize=1)
def get_model() -> object:
    return load_model_from_disk()
```

### Timeout around external API calls

```python
import httpx

async def call_api(url: str) -> dict:
    async with httpx.AsyncClient(timeout=httpx.Timeout(10.0)) as client:
        response = await client.get(url)
        response.raise_for_status()
        return response.json()
```

---

## Output Style

When completing Python work, report:

- files changed
- behavior changed
- dependency or environment changes
- checks run and result
- any risk involving data, model contracts, migrations, async behavior, or production deployment

Keep summaries concrete. Avoid vague claims like “optimized” or “made robust” unless the exact mechanism is stated.
