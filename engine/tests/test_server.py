import json
import urllib.error
import urllib.request


def call(url, body=None, token="", owner="local", method=None):
    data = json.dumps(body).encode() if body is not None else None
    request = urllib.request.Request(
        url,
        data=data,
        method=method or ("POST" if data else "GET"),
        headers={"Content-Type": "application/json", "Authorization": "Bearer " + token, "X-Catena-Owner": owner},
    )
    try:
        with urllib.request.urlopen(request, timeout=5) as r:
            return r.status, json.loads(r.read())
    except urllib.error.HTTPError as e:
        return e.code, json.loads(e.read())
