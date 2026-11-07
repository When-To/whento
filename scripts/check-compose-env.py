#!/usr/bin/env python3
# WhenTo - Collaborative event calendar for self-hosted environments
# Copyright (C) 2025 WhenTo Contributors
# SPDX-License-Identifier: BSL-1.1
"""Check that backend-only settings reach only the backend Compose services.

Runs `docker compose config` against a fixture .env: no containers start and the
real .env is never read.
"""

import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parent.parent
# Compose file -> the service running the backend (None: no backend service).
COMPOSE_FILES = (
    ("docker-compose.yml", "app"),
    (".devcontainer/docker-compose.yml", "devcontainer"),
    ("docker-compose.dev.yml", None),
)
REMINDER_DEFAULTS = {
    "REMINDER_HOURS_BEFORE": "24",
    "REMINDER_INTERVAL": "5m",
    "REMINDER_CATCH_UP_WINDOW": "15m",
    "REMINDER_MAX_ATTEMPTS": "5",
    "REMINDER_RETRY_BACKOFF": "1m",
}
HOLIDAY = "HOLIDAYS_NAGER_FALLBACK_ENABLED"
BOOTSTRAP = ("BOOTSTRAP_KEY", "BOOTSTRAP_KEY_FILE")


def compose_services(tmp, fixture_text):
    """Yield (compose, backend, services) for every Compose file and the given .env."""
    # Synthetic credentials; settings under test must come from the fixture, not the shell.
    env = dict(os.environ)
    for name in (*REMINDER_DEFAULTS, HOLIDAY, *BOOTSTRAP, "COMPOSE_FILE", "COMPOSE_ENV_FILES"):
        env.pop(name, None)
    env.update(
        DB_USER="whento", DB_NAME="whento", DB_PASSWORD="compose-test-only",
        REDIS_PASSWORD="compose-test-only", APP_ENV="development",
        APP_URL="http://localhost:8080", SMTP_HOST="", SMTP_USERNAME="",
        SMTP_PASSWORD="", SMTP_FROM="",
    )
    fixture = Path(tmp) / ".env"
    fixture.write_text(fixture_text, encoding="utf-8")
    for compose, backend in COMPOSE_FILES:
        result = subprocess.run(
            ["docker", "compose", "--env-file", str(fixture),
             "-f", str(ROOT / compose), "config", "--format", "json"],
            cwd=ROOT, env=env, capture_output=True, text=True, timeout=30, check=True,
        )
        yield compose, backend, json.loads(result.stdout)["services"]


class ComposeEnvTests(unittest.TestCase):
    def check(self, label, fixture_text, expected):
        """expected: setting -> value the backend service must see (None = unset)."""
        with tempfile.TemporaryDirectory(prefix="whento-compose-") as tmp:
            for compose, backend, services in compose_services(tmp, fixture_text):
                with self.subTest(case=label, compose=compose):
                    if backend is None:
                        self.assertEqual(set(services), {"postgres", "redis"})
                    else:
                        self.assertIn(backend, services)
                    for name, service in services.items():
                        settings = service.get("environment", {})
                        for key, value in expected.items():
                            if name == backend:
                                self.assertEqual(settings.get(key), value)
                            else:
                                self.assertNotIn(key, settings)

    def test_bootstrap(self):
        for label, key, secret_file in (
            ("unset", "", ""),
            ("pinned", "compose-test-only-boot-key-0123456789abcdef", ""),
            ("file", "", "/run/secrets/bootstrap_key"),
        ):
            self.check(f"bootstrap-{label}",
                       f"BOOTSTRAP_KEY={key}\nBOOTSTRAP_KEY_FILE={secret_file}\n",
                       {"BOOTSTRAP_KEY": key, "BOOTSTRAP_KEY_FILE": secret_file})

    def test_reminder_defaults_and_overrides(self):
        overrides = dict(zip(REMINDER_DEFAULTS, ("48", "10m", "30m", "8", "2m")))
        self.check("reminder-defaults", "", REMINDER_DEFAULTS)
        self.check("reminder-overrides",
                   "".join(f"{k}={v}\n" for k, v in overrides.items()), overrides)

    def test_holiday_fallback_is_opt_in(self):
        for value in (None, "false", "true"):
            self.check(f"holiday-{value}", "" if value is None else f"{HOLIDAY}={value}\n",
                       {HOLIDAY: value or "false"})


if __name__ == "__main__":
    unittest.main(verbosity=2)
