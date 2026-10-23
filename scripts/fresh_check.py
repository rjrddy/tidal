"""Independent clone, real prerequisites and actual make demo, no borrowed artifacts."""

import json
import os
import signal
import subprocess
import tempfile
import time
import urllib.request
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
OUT = ROOT / "docs/results/fresh-clone"


def main() -> None:
    OUT.mkdir(parents=True, exist_ok=True)
    head = subprocess.check_output(["git", "rev-parse", "HEAD"], cwd=ROOT, text=True).strip()
    with tempfile.TemporaryDirectory(prefix="tidal-fresh-") as temp:
        clone = Path(temp) / "tidal"
        subprocess.run(
            ["git", "clone", "--no-hardlinks", str(ROOT), str(clone)],
            check=True,
            capture_output=True,
        )
        with (OUT / "checks.log").open("w") as log:
            subprocess.run(
                ["make", "setup", "lint", "test", "bench"],
                cwd=clone,
                stdout=log,
                stderr=log,
                check=True,
            )
        for file in ["campaign.json", "core-bench.log", "tests.log"]:
            (OUT / file).write_bytes((clone / "docs/results" / file).read_bytes())
        with (OUT / "demo.log").open("w") as log:
            child = subprocess.Popen(
                ["make", "demo"],
                cwd=clone,
                env={**os.environ, "TIDAL_ADDR": "127.0.0.1:18100"},
                stdout=log,
                stderr=log,
                start_new_session=True,
            )
            try:
                for _ in range(600):
                    if child.poll() is not None:
                        raise RuntimeError((OUT / "demo.log").read_text())
                    try:
                        with urllib.request.urlopen(
                            "http://127.0.0.1:18100/", timeout=1
                        ) as response:
                            assert b"Tidal" in response.read()
                        break
                    except OSError:
                        time.sleep(0.2)
                else:
                    raise RuntimeError("make demo readiness timeout")
                receipts = []
                for _ in range(2):
                    request = urllib.request.Request(
                        "http://127.0.0.1:18100/api/get?key=k17",
                        headers={"Authorization": "Bearer local-demo"},
                    )
                    with urllib.request.urlopen(request, timeout=5) as response:
                        receipts.append(json.load(response))
                assert not receipts[0]["result"]["hit"] and receipts[1]["result"]["hit"]
                (OUT / "verification.json").write_text(
                    json.dumps(
                        {
                            "head": head,
                            "commands": [
                                "git clone --no-hardlinks",
                                "make setup lint test bench",
                                "make demo",
                            ],
                            "copied_ignored_artifacts": False,
                            "public_trace": "tracked licensed fixture, SHA256 verified",
                            "live_http_receipts": receipts,
                            "passed": True,
                        },
                        indent=2,
                    )
                    + "\n"
                )
                print(json.dumps({"head": head, "fresh_clone": "pass", "actual_make_demo": "pass"}))
            finally:
                os.killpg(child.pid, signal.SIGTERM)
                child.wait(timeout=10)


if __name__ == "__main__":
    main()
