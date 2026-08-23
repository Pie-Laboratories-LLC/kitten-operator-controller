import os
import re
import requests
from flask import Flask, Response, request

app = Flask(__name__)

UPSTREAM_URL = os.environ.get("UPSTREAM_URL", "http://localhost:8001")
KITTEN_SERVICE_URL = os.environ.get("KITTEN_SERVICE_URL", "http://kitten-operator/kittenpictures")

BODY_TAG_RE = re.compile(rb"(<body[^>]*>)", re.IGNORECASE)


def fetch_kitten_url():
    resp = requests.get(KITTEN_SERVICE_URL, params={"format": "json"}, timeout=3)
    resp.raise_for_status()
    return resp.json()["url"]


def inject_kitten(html_bytes):
    try:
        kitten_url = fetch_kitten_url()
    except requests.RequestException as e:
        print(f"kitten fetch failed: {e}", flush=True)
        # Fail open: if the kitten service is unreachable, pass the page
        # through unmodified rather than breaking the actual application.
        return html_bytes

    img_tag = f'<img src="{kitten_url}" alt="a kitten, unconditionally" />'.encode()
    return BODY_TAG_RE.sub(lambda m: m.group(1) + img_tag, html_bytes, count=1)


@app.route("/", defaults={"path": ""}, methods=["GET", "POST", "PUT", "DELETE", "PATCH"])
@app.route("/<path:path>", methods=["GET", "POST", "PUT", "DELETE", "PATCH"])
def proxy(path):
    upstream_url = f"{UPSTREAM_URL}/{path}"

    upstream_resp = requests.request(
        method=request.method,
        url=upstream_url,
        headers={k: v for k, v in request.headers if k.lower() != "host"},
        params=request.args,
        data=request.get_data(),
        timeout=10,
        allow_redirects=False,
    )

    content_type = upstream_resp.headers.get("Content-Type", "")
    body = upstream_resp.content

    if content_type.startswith("text/html"):
        body = inject_kitten(body)

    excluded_headers = {"content-encoding", "content-length", "transfer-encoding", "connection"}
    headers = [(k, v) for k, v in upstream_resp.headers.items() if k.lower() not in excluded_headers]

    return Response(body, status=upstream_resp.status_code, headers=headers)


@app.route("/healthz")
def healthz():
    return {"status": "ok"}, 200

if __name__ == "__main__": app.run(host="0.0.0.0", port=8000)
