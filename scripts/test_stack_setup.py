#!/usr/bin/env python3
"""Regression checks use temporary private directories, never repository secrets.
Run: python3 -m unittest discover -s scripts -p 'test_stack_setup.py' -v
"""
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch

import stack_setup as setup


class SetupTests(unittest.TestCase):
    def test_prepare_is_private_and_no_clobber(self):
        with tempfile.TemporaryDirectory() as tmp:
            directory = Path(tmp) / "dev"
            config = directory / "garage.toml"
            setup.prepare(directory, config)
            original = {p.name: p.read_bytes() for p in directory.iterdir()}
            setup.prepare(directory, config)
            self.assertEqual(original, {p.name: p.read_bytes() for p in directory.iterdir()})
            self.assertEqual(directory.stat().st_mode & 0o777, 0o700)
            for p in directory.iterdir():
                self.assertEqual(p.stat().st_mode & 0o777, 0o600)
            self.assertNotIn("s3_access_key", original)
            self.assertNotIn("rpc_secret =", config.read_text())

    def test_empty_or_symlink_credentials_fail_closed(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp) / "secret"
            path.touch(mode=0o600)
            with self.assertRaises(RuntimeError):
                setup.publish(path, "replacement")
            path.unlink()
            target = Path(tmp) / "target"
            target.write_text("original")
            path.symlink_to(target)
            with self.assertRaises(RuntimeError):
                setup.publish(path, "replacement")
            self.assertEqual(target.read_text(), "original")

    def test_mismatched_s3_credentials_are_not_replaced(self):
        with tempfile.TemporaryDirectory() as tmp, patch.dict(os.environ, {"SECRET_DIR": tmp}):
            stack = setup.Stack("dev")
            key = Path(tmp) / "s3_access_key"
            setup.publish(key, "GK0011\n")
            calls = []

            def garage(*args, **kwargs):
                calls.append(args)
                output = ""
                if args[:2] == ("layout", "show"):
                    output = "Current cluster layout version: 1\n"
                if args[:2] == ("key", "info"):
                    output = "Key ID: GK0022\nSecret key: aabb\n"
                return subprocess.CompletedProcess(args, 0, output, "")

            stack.garage = garage
            with self.assertRaisesRegex(RuntimeError, "differs"):
                stack.bootstrap()
            self.assertEqual(key.read_text(), "GK0011\n")
            self.assertNotIn(("key", "create", "klubhub-app-key"), calls)

    def test_compose_contracts_with_real_compose(self):
        configs = {}
        # Use only explicit fixture env, not the user's .env or exported credentials.
        env = {k: v for k, v in os.environ.items() if k in ("PATH", "HOME", "DOCKER_HOST", "DOCKER_CONTEXT")}
        with tempfile.TemporaryDirectory() as tmp:
            env.update(SECRET_DIR=tmp, GARAGE_CONFIG_FILE=str(Path(tmp) / "garage.toml"))
            for mode, files in (("dev", ["docker-compose.yml"]),
                                ("debug", ["docker-compose.yml", "docker-compose.dev.yml"]),
                                ("prod", ["docker-compose.prod.yml"])):
                command = ["docker", "compose", "--env-file", "/dev/null"]
                for file in files:
                    command.extend(["-f", str(setup.ROOT / file)])
                result = subprocess.run(command + ["config", "--format", "json"], env=env,
                                        check=True, capture_output=True, text=True)
                configs[mode] = json.loads(result.stdout)
        for config in configs.values():
            self.assertEqual(set(config["services"]), {"app", "db", "storage"})
            app = config["services"]["app"]
            self.assertEqual(app["environment"]["BIND_ADDRESS"], "0.0.0.0")
            self.assertEqual(app["ports"][0]["host_ip"], "127.0.0.1")
            self.assertEqual(app["depends_on"]["storage"]["condition"], "service_healthy")
            for service in config["services"].values():
                self.assertNotIn("container_name", service)
        dev, prod = configs["dev"], configs["prod"]
        self.assertNotEqual(dev["name"], prod["name"])
        self.assertNotEqual(dev["volumes"]["db_data"]["name"], prod["volumes"]["db_data"]["name"])
        self.assertTrue(dev["services"]["app"]["build"]["context"].endswith("/api"))
        self.assertEqual(dev["services"]["app"]["environment"]["NUXT_INTERNAL_URL"], "http://host.docker.internal:4200")
        self.assertNotIn("build", prod["services"]["app"])
        self.assertNotIn("ports", prod["services"]["db"])
        self.assertEqual(prod["services"]["app"]["image"], "ghcr.io/lithqube/klubhub-dj-api:v1.0.1")
        self.assertEqual(prod["services"]["app"]["environment"]["SERVE_FRONTEND"], "true")
        admin = [p for p in configs["debug"]["services"]["storage"]["ports"] if p["target"] == 3903]
        self.assertEqual(len(admin), 1)
        self.assertEqual(admin[0]["host_ip"], "127.0.0.1")

    def test_email_overlay_wires_plunk_when_email_env_present(self):
        """Plunk overlay must not break the existing dev/prod configs.

        Run `docker compose config` with the email overlay applied and
        assert the plunk service shows up under the 'email' profile. The
        base dev/prod compose files (without the overlay) must remain
        unchanged.
        """
        with tempfile.TemporaryDirectory() as tmp:
            env_file = Path(tmp) / "email.env"
            env_file.write_text(
                "PLUNK_AUTH_SECRET=test-only-not-real\n"
                "POSTGRES_PASSWORD=test-only-not-real\n"
            )
            base_env = {k: v for k, v in os.environ.items() if k in ("PATH", "HOME", "DOCKER_HOST", "DOCKER_CONTEXT")}
            base_env.update(
                SECRET_DIR=tmp,
                GARAGE_CONFIG_FILE=str(Path(tmp) / "garage.toml"),
                ENV_FILE=str(Path(tmp) / ".env"),
                POSTGRES_PASSWORD="test-only-not-real",
                POSTGRES_USER="klubhub",
                POSTGRES_HOST="db",
                POSTGRES_PORT="5432",
                POSTGRES_DB="klubhub",
                PLUNK_AUTH_SECRET="test-only-not-real",
            )
            command = [
                "docker", "compose", "--env-file", "/dev/null",
                "-f", str(setup.ROOT / "docker-compose.yml"),
                "-f", str(setup.ROOT / "docker-compose.email.yml"),
                "--profile", "email",
                "config", "--format", "json",
            ]
            try:
                result = subprocess.run(
                    command, env=base_env, check=True, capture_output=True, text=True,
                )
            except subprocess.CalledProcessError as exc:
                self.fail(
                    "compose config failed:\nstdout=%s\nstderr=%s"
                    % (exc.stdout, exc.stderr)
                )
            config = json.loads(result.stdout)
            services = config.get("services", {})
            self.assertIn("plunk", services, "plunk service missing from email overlay")
            self.assertIn("app", services, "api service missing under email profile")
            api_env = services["app"]["environment"]
            self.assertIn("PLUNK_BASE_URL", api_env)
            self.assertEqual(api_env["PLUNK_BASE_URL"], "http://plunk:3000")
            self.assertEqual(api_env["PLUNK_FROM_EMAIL"], "noreply@klubhub.local")


if __name__ == "__main__":
    unittest.main()
