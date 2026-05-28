from fastapi import FastAPI
from pydantic import BaseModel
from strategy_validator import validate_strategy

app = FastAPI()


class ValidateRequest(BaseModel):
    source: str


@app.post("/validate")
def validate(req: ValidateRequest):
    return validate_strategy(req.source)


@app.get("/health")
def health():
    return {"status": "ok"}
