"""Seed a running local demo using real authenticated HTTP cache requests."""

import json
import os
import urllib.request

base = os.environ.get("TIDAL_URL", "http://127.0.0.1:8095")
token = os.environ.get("TIDAL_TOKEN", "local-demo")
request = urllib.request.Request(
    base + "/api/drive",
    data=b'{"trace":"hotset","count":1024}',
    headers={"Authorization": "Bearer " + token, "Content-Type": "application/json"},
)
with urllib.request.urlopen(request, timeout=30) as response:
    print(
        json.dumps(
            {
                "scope": "real HTTP cache calls on seeded synthetic key demand",
                "cache": json.load(response),
            },
            indent=2,
        )
    )
