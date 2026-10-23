"""Read-only formatting gate using the pinned Go toolchain's gofmt."""

import subprocess
from pathlib import Path

root = Path(subprocess.check_output(["go", "env", "GOROOT"], text=True).strip())
files = [str(p) for directory in ["cmd", "internal"] for p in Path(directory).rglob("*.go")]
result = subprocess.run(
    [str(root / "bin/gofmt"), "-l", *files], check=True, capture_output=True, text=True
)
if result.stdout:
    raise SystemExit("gofmt required:\n" + result.stdout)
print("Go formatting clean")
