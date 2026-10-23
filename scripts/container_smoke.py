"""Exercise the actual nonroot image, bearer auth, native hit, invalidation and failure."""

import json
import subprocess
import time
import urllib.error
import urllib.request

TOKEN = "synthetic-container-token-123456789"
NAME = "tidal-smoke"


def docker(*args: str, check: bool = True) -> str:
    r = subprocess.run(["docker", *args], capture_output=True, text=True, check=check)
    return (r.stdout + r.stderr if args[0] == "logs" else r.stdout).strip()


def request(path: str, body=None, token=TOKEN):
    req = urllib.request.Request(
        "http://127.0.0.1:18099" + path,
        data=json.dumps(body).encode() if body is not None else None,
        headers={"Authorization": "Bearer " + token, "Content-Type": "application/json"},
    )
    try:
        with urllib.request.urlopen(req, timeout=10) as response:
            return response.status, json.load(response)
    except urllib.error.HTTPError as error:
        return error.code, json.load(error)


def main() -> None:
    try:
        docker(
            "run",
            "-d",
            "--name",
            NAME,
            "-p",
            "127.0.0.1:18099:8095",
            "-e",
            "TIDAL_TOKEN=" + TOKEN,
            "tidal:test",
        )
        for _ in range(100):
            try:
                if request("/health")[0] == 200:
                    break
            except OSError:
                time.sleep(0.1)
        assert request("/api/state", token="wrong")[0] == 401
        first = request("/api/get?key=k17")[1]
        second = request("/api/get?key=k17")[1]
        assert not first["result"]["hit"] and second["result"]["hit"]
        assert request("/api/mutate", {"key": "k17", "invalidate": True})[0] == 200
        fresh = request("/api/get?key=k17")[1]
        assert fresh["result"]["value"]["version"] == 1 and not fresh["stale_observed"]
        request("/api/fault", {"fail": True})
        assert request("/api/get?key=k99999")[0] == 502
        request("/api/fault", {"fail": False})
        assert request("/api/get?key=k99999")[0] == 200
        docker("restart", NAME)
        for _ in range(100):
            try:
                if request("/health")[0] == 200:
                    break
            except OSError:
                time.sleep(0.1)
        assert request("/api/get?key=k17")[0] == 200
        print(
            json.dumps(
                {
                    "image": "distroless nonroot",
                    "http_origin_hit_invalidation_outage_recovery_restart": "pass",
                    "state_persistence": "not promised; cache is intentionally disposable",
                }
            )
        )
    finally:
        docker("logs", NAME, check=False)
        docker("rm", "-f", NAME, check=False)


if __name__ == "__main__":
    main()
