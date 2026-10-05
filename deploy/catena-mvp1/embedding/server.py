"""Private OpenAI-compatible CPU embeddings for OpenViking."""
import json
import os
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from threading import Lock

from fastembed import TextEmbedding

MODEL = os.getenv("CATENA_EMBEDDING_MODEL", "BAAI/bge-small-zh-v1.5")
options = {"specific_model_path": os.environ["CATENA_EMBEDDING_MODEL_PATH"]} if os.getenv("CATENA_EMBEDDING_MODEL_PATH") else {}
embedder = TextEmbedding(model_name=MODEL, cache_dir="/models", threads=2, **options)
inference_lock = Lock()


class Handler(BaseHTTPRequestHandler):
    def reply(self, status, body):
        data = json.dumps(body, ensure_ascii=False).encode()
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)

    def do_GET(self):
        if self.path == "/health":
            self.reply(200, {"status": "ready", "model": MODEL})
        else:
            self.reply(404, {"error": "not found"})

    def do_POST(self):
        if self.path != "/v1/embeddings":
            return self.reply(404, {"error": "not found"})
        try:
            length = int(self.headers.get("Content-Length", "0"))
            if length <= 0 or length > 1024 * 1024:
                return self.reply(413, {"error": "input exceeds 1 MiB"})
            payload = json.loads(self.rfile.read(length))
            texts = payload.get("input")
            if isinstance(texts, str):
                texts = [texts]
            if not isinstance(texts, list) or not 1 <= len(texts) <= 128 or not all(isinstance(t, str) and t.strip() for t in texts):
                return self.reply(400, {"error": "input must contain 1-128 nonempty strings"})
            if payload.get("model", MODEL) != MODEL:
                return self.reply(400, {"error": "unknown model"})
            if payload.get("dimensions", 512) != 512 or payload.get("encoding_format", "float") != "float":
                return self.reply(400, {"error": "this model returns 512-dimensional float vectors"})
            with inference_lock:
                vectors = list(embedder.embed(texts, batch_size=16))
            self.reply(200, {"object": "list", "model": MODEL, "data": [
                {"object": "embedding", "index": i, "embedding": v.tolist()}
                for i, v in enumerate(vectors)
            ], "usage": {"prompt_tokens": 0, "total_tokens": 0}})
        except (ValueError, TypeError):
            self.reply(400, {"error": "invalid request"})
        except Exception:
            self.reply(500, {"error": "embedding failed"})


ThreadingHTTPServer(("0.0.0.0", 8080), Handler).serve_forever()
