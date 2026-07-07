import base64
from pathlib import Path

import httpx
from pydantic import BaseModel


class ImageData(BaseModel):
    image: str  # base64 encoded image

    @classmethod
    def from_img_path(cls, img_path: Path):
        img_bytes = Path(img_path).read_bytes()
        img_b64 = base64.b64encode(img_bytes).decode("utf-8")
        return cls(image=img_b64)


class PredictResponse(BaseModel):
    image: str | None  # base64 encoded image
    roundness: float | None
    labels: dict[str, float] | None  # Labels with confidences

    def save_img(self, out_path: Path):
        if self.image is None:
            raise ValueError("No image to save.")
        out_path.parent.mkdir(parents=True, exist_ok=True)
        img_bytes = base64.b64decode(self.image)
        out_path.write_bytes(img_bytes)


class PredictionError(Exception): ...


class InferenceClient:
    def __init__(self, base_url: str):
        self.client = lambda: httpx.AsyncClient(base_url=base_url, timeout=30)

    async def predict(self, payload: ImageData) -> PredictResponse:
        async with self.client() as client:
            res = await client.post("/predict/predict", json=payload.model_dump())
            if res.status_code != 200:
                raise PredictionError()
            return PredictResponse.model_validate(res.json())
