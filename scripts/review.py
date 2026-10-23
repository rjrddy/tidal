"""Real browser checks and an optional unpadded recording of changed cache requests."""

import argparse
import json
import subprocess
import time
import urllib.request
from pathlib import Path

from playwright.sync_api import expect, sync_playwright

ROOT = Path(__file__).resolve().parents[1]
OUT = ROOT / "docs/results/browser"


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--record", action="store_true")
    args = parser.parse_args()
    OUT.mkdir(parents=True, exist_ok=True)
    errors, receipts = [], []
    with (OUT / "server.log").open("w") as log:
        process = subprocess.Popen(
            [str(ROOT / "bin/tidal"), "-addr", "127.0.0.1:18098"], cwd=ROOT, stdout=log, stderr=log
        )
        try:
            for _ in range(100):
                if process.poll() is not None:
                    raise RuntimeError((OUT / "server.log").read_text())
                try:
                    urllib.request.urlopen("http://127.0.0.1:18098/health", timeout=1).close()
                    break
                except OSError:
                    time.sleep(0.1)
            with sync_playwright() as p:
                browser = p.chromium.launch(executable_path=p.chromium.executable_path)
                context = browser.new_context(
                    viewport={"width": 1440, "height": 1100},
                    **(
                        {
                            "record_video_dir": str(OUT / "recording"),
                            "record_video_size": {"width": 1440, "height": 1100},
                        }
                        if args.record
                        else {}
                    ),
                )
                page = context.new_page()
                page.on("pageerror", lambda e: errors.append(str(e)))
                page.goto("http://127.0.0.1:18098")
                expect(page.get_by_text("LOCAL LAB CONNECTED")).to_be_visible()
                started = time.monotonic()
                with page.expect_response(lambda r: r.url.endswith("/api/replay")) as response:
                    page.get_by_role("button", name="Run 4,096 requests").click()
                data = response.value.json()
                assert response.value.ok and len(data["rows"]) == 5
                expect(page.get_by_role("img", name="Hit rate by policy over time")).to_be_visible()
                receipts.append(
                    {
                        "kind": "replay",
                        "rows": [
                            {
                                k: r[k]
                                for k in [
                                    "trace",
                                    "mode",
                                    "requests",
                                    "hit_rate",
                                    "origin_calls",
                                    "stale_serves",
                                    "switches",
                                ]
                            }
                            for r in data["rows"]
                        ],
                    }
                )
                page.screenshot(path=str(OUT / "desktop.png"), full_page=True)
                page.get_by_role("button", name="Recorded runs").click()
                page.get_by_label("Traffic pattern", exact=True).select_option("web12")
                expect(page.get_by_text("95,607 requests / this trace")).to_be_visible()
                page.screenshot(path=str(OUT / "evidence.png"), full_page=True)
                page.get_by_role("button", name="Live cache", exact=False).click()
                page.get_by_label("Synthetic key").fill("k90177")

                def query() -> dict:
                    with page.expect_response(lambda r: "/api/get?" in r.url) as response:
                        page.get_by_role("button", name="Fetch key").click()
                    value = response.value.json()
                    assert response.value.ok, value
                    expect(page.locator(".query-receipt")).to_be_visible()
                    receipts.append({"kind": "live-query", **value})
                    return value

                first = query()
                assert not first["result"]["hit"]
                second = query()
                assert second["result"]["hit"] and not second["stale_observed"]
                page.get_by_label("Deliver invalidation after source change").uncheck()
                page.get_by_role("button", name="Change origin version").click()
                expect(page.get_by_role("status")).to_contain_text("withheld")
                stale = query()
                assert stale["stale_observed"]
                page.get_by_label("Deliver invalidation after source change").check()
                page.get_by_role("button", name="Change origin version").click()
                expect(page.get_by_role("status")).to_contain_text("delivered")
                fresh = query()
                assert not fresh["stale_observed"] and fresh["result"]["value"]["version"] == 2
                page.get_by_role("button", name="Inject an origin outage").click()
                expect(page.get_by_role("status")).to_contain_text("fault is active")
                page.get_by_label("Synthetic key").fill("k99999")
                page.get_by_role("button", name="Fetch key").click()
                expect(page.get_by_role("alert")).to_contain_text("origin status 503")
                expect(page.locator(".query-receipt")).to_have_count(0)
                receipts.append(
                    {"kind": "caught-origin-outage", "text": page.get_by_role("alert").inner_text()}
                )
                page.get_by_role("button", name="Recover it", exact=False).click()
                expect(page.get_by_role("status")).to_contain_text("recovered")
                query()
                page.get_by_label("Traffic pattern", exact=True).select_option("scan")
                with page.expect_response(lambda r: r.url.endswith("/api/drive")) as response:
                    page.get_by_role("button", name="Send 1,024 requests").click()
                assert response.value.ok
                expect(page.get_by_text("last completed window").first).to_be_visible()
                receipts.append({"kind": "live-drive", "cache": response.value.json()})
                page.get_by_label("Synthetic key").fill("invalid key")
                page.get_by_role("button", name="Fetch key").click()
                expect(page.get_by_role("alert")).to_contain_text("key must be")
                page.get_by_label("Synthetic key").fill("k17")
                query()
                page.locator("summary").click()
                page.get_by_label("Bearer token").fill("wrong-token")
                page.get_by_role("button", name="Reconnect", exact=True).click()
                expect(page.get_by_role("alert")).to_contain_text("bearer required")
                page.get_by_label("Bearer token").fill("local-demo")
                page.get_by_role("button", name="Reconnect", exact=True).click()
                expect(page.get_by_role("alert")).to_have_count(0)
                expect(page.get_by_text("LOCAL LAB CONNECTED")).to_be_visible()
                for width in [390, 768, 1024, 1440]:
                    page.set_viewport_size({"width": width, "height": 1100})
                    assert page.evaluate("document.documentElement.scrollWidth <= innerWidth"), (
                        width
                    )
                    expect(page.get_by_role("button", name="Fetch key")).to_be_visible()
                    if width == 390:
                        page.screenshot(path=str(OUT / "mobile.png"), full_page=True)
                    receipts.append({"kind": "responsive", "width": width, "no_overflow": True})
                index = 0
                while args.record and time.monotonic() - started < 92:
                    page.get_by_label("Synthetic key").fill(f"k{100 + index % 20}")
                    query()
                    if index % 4 == 0:
                        page.get_by_role("button", name="Change origin version").click()
                        expect(page.get_by_role("status")).to_contain_text("delivered")
                        query()
                    page.wait_for_timeout(1200)
                    index += 1
                page.get_by_role("button", name="Fetch key").focus()
                expect(page.get_by_role("button", name="Fetch key")).to_be_focused()
                assert not errors, errors
                elapsed = time.monotonic() - started
                video = page.video
                context.close()
                if video:
                    video.save_as(str(OUT / "tidal-demo.webm"))
                    video.delete()
                browser.close()
                (OUT / "review.json").write_text(
                    json.dumps(
                        {"elapsed_seconds": elapsed, "page_errors": errors, "receipts": receipts},
                        indent=2,
                    )
                    + "\n"
                )
                print(
                    json.dumps(
                        {
                            "elapsed_seconds": elapsed,
                            "receipts": len(receipts),
                            "page_errors": errors,
                        }
                    )
                )
        finally:
            process.terminate()
            process.wait(timeout=10)


if __name__ == "__main__":
    main()
