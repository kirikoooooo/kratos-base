# LangChainGo CLIP Image-Text Demo

The demo loads the Chinese-capable `OFA-Sys/chinese-clip-vit-base-patch16` once
in a local Python service.
It encodes image and text with the same checkpoint, L2-normalizes both vectors,
then uses Go to calculate cosine similarity. the local CLIP service provides
the service's text endpoint; LangChainGo does not provide a CLIP image embedder.

## Start the local service

```bash
cd third_party/clipdemo
uv venv
uv pip install -r requirements.txt
uv run uvicorn clip_server:app --host 127.0.0.1 --port 8088
```

The first start downloads the Hugging Face model. Override the checkpoint or
device if needed:

```bash
CLIP_MODEL_ID=OFA-Sys/chinese-clip-vit-large-patch14 CLIP_DEVICE=mps \
  uv run uvicorn clip_server:app --host 127.0.0.1 --port 8088
```

## Run the Go verification

```bash
export CLIP_BASE_URL=http://127.0.0.1:8088
export CLIP_IMAGE=/Users/lixiongfei/Code/Go/kratos-base/third_party/clipdemo/test.png
go run ./third_party/clipdemo/cmd/clipdemo -- "a photo of a dog" "a photo of a car"
```

The command prints one score per text. A semantically matching description
should rank above unrelated text; absolute values vary by image and checkpoint.
