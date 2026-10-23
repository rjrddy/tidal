"""Verify and summarize the complete public key-order trace; no private fields."""

import gzip
import hashlib
import json
import struct
from pathlib import Path

root = Path(__file__).resolve().parents[1]
p = root / "data/web12.bin.gz"
sha = hashlib.sha256(p.read_bytes()).hexdigest()
assert sha == "30e5ec69a7e1a1789f17d788e4aab61146dce68a449fb29779ead89da944e4c9"
raw = gzip.decompress(p.read_bytes())
assert len(raw) % 4 == 0
keys = [row[0] for row in struct.iter_unpack(">I", raw)]
result = {"name": "Web12", "upstream_commit": "738948d580e757377e755f0c3bfa2ac8f818ffcd", "url": "https://github.com/cache2k/cache2k-benchmark/blob/738948d580e757377e755f0c3bfa2ac8f818ffcd/traces/src/main/resources/org/cache2k/benchmark/traces/trace-mt-20121220.trc.bin.gz", "copyright": "headissue GmbH, Jens Wilke", "license": "CC BY 4.0; data/LICENSE-public-trace.txt", "sha256": sha, "requests": len(keys), "unique_keys": len(set(keys)), "decode": "gzip, unsigned big-endian 32-bit keys, original order, entire file", "synthetic_annotations": "10ms per request; version changes and invalidations generated separately; no original times, values, object sizes or freshness metadata supplied"}
(root / "docs/results/trace-provenance.json").write_text(json.dumps(result, indent=2) + "\n")
print(json.dumps(result, indent=2))
