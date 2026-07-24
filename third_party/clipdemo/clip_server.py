"""Local CLIP encoders for the standalone Go similarity demo."""

import base64
import io
import os

import torch
from fastapi import FastAPI, HTTPException
from pydantic import BaseModel
from PIL import Image
from transformers import ChineseCLIPModel, ChineseCLIPProcessor

MODEL_ID = os.getenv("CLIP_MODEL_ID", "OFA-Sys/chinese-clip-vit-base-patch16")
DEVICE = os.getenv("CLIP_DEVICE", "mps" if torch.backends.mps.is_available() else "cpu")

app = FastAPI(title="clipdemo")
model = ChineseCLIPModel.from_pretrained(MODEL_ID).to(DEVICE).eval()
processor = ChineseCLIPProcessor.from_pretrained(MODEL_ID)


class TextRequest(BaseModel):
    texts: list[str]


class ImageRequest(BaseModel):
    image_base64: str


def normalized(features: torch.Tensor) -> list[float]:
    return torch.nn.functional.normalize(features, p=2, dim=-1)[0].cpu().tolist()


@app.post("/embed/text")
def embed_text(request: TextRequest):
    if not request.texts:
        raise HTTPException(400, "texts is required")
    inputs = processor(text=request.texts, return_tensors="pt", padding=True, truncation=True).to(DEVICE)
    with torch.inference_mode():
        features = model.get_text_features(**inputs)
        features = torch.nn.functional.normalize(features, p=2, dim=-1).cpu().tolist()
    return {"embeddings": features, "model": MODEL_ID}


@app.post("/embed/image")
def embed_image(request: ImageRequest):
    try:
        image = Image.open(io.BytesIO(base64.b64decode(request.image_base64))).convert("RGB")
    except Exception as exc:
        raise HTTPException(400, f"invalid image: {exc}") from exc
    inputs = processor(images=image, return_tensors="pt").to(DEVICE)
    with torch.inference_mode():
        features = model.get_image_features(**inputs)
    return {"embedding": normalized(features), "model": MODEL_ID}
