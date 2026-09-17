#!/usr/bin/env python3
"""Safe single-node setup. Requires Python 3, Docker Compose v2+, Garage v2.2.

No dotenv loading or shell evaluation. Existing files are never replaced.
SECRET_DIR/GARAGE_CONFIG_FILE/COMPOSE_PROJECT_NAME and other explicitly exported
Compose variables are honored. Defaults are separate, gitignored secrets/dev and
secrets/prod directories. --prepare-only needs no Docker daemon.
"""
import argparse
import fcntl
import os
from pathlib import Path
import re
import secrets
import stat
import subprocess
import sys
import tempfile
import time

ROOT = Path(__file__).resolve().parent.parent


def private_dir(path):
    path.mkdir(mode=0o700, parents=True, exist_ok=True)
    if path.is_symlink() or not path.is_dir():
        raise RuntimeError("Secret directory must be a real directory")
    if stat.S_IMODE(path.stat().st_mode) & 0o077:
        raise RuntimeError("Secret directory must be private (chmod 700)")


def publish(path, value):
    """Publish a fully written file without replacing any existing pathname."""
    if path.is_symlink():
        raise RuntimeError("Refusing a symlink in setup files")
    if path.exists():
        if not path.is_file() or path.stat().st_size == 0:
            raise RuntimeError("Existing setup file is empty or not a regular file")
        if stat.S_IMODE(path.stat().st_mode) & 0o077:
            raise RuntimeError("Existing setup file must be private (chmod 600)")
        return
    fd, temp = tempfile.mkstemp(prefix=".setup-", dir=path.parent)
    try:
        with os.fdopen(fd, "w") as handle:
            handle.write(value)
            handle.flush()
            os.fsync(handle.fileno())
        os.link(temp, path)  # atomic no-clobber; concurrent writers fail closed
    finally:
        os.unlink(temp)


def prepare(secret_dir, config):
    private_dir(secret_dir)
    for name in ("postgres_password", "garage_rpc_secret", "garage_admin_token",
                 "token_encryption_key", "ical_secret"):
        publish(secret_dir / name, secrets.token_hex(32) + "\n")
    private_dir(config.parent)
    publish(config, (ROOT / "garage.toml.example").read_text())


class Stack:
    def __init__(self, mode, compose_file=None):
        self.mode = mode
        self.secret_dir = Path(os.environ.get("SECRET_DIR", ROOT / "secrets" / mode)).absolute()
        self.config = Path(os.environ.get("GARAGE_CONFIG_FILE", self.secret_dir / "garage.toml")).absolute()
        self.env = dict(os.environ, SECRET_DIR=str(self.secret_dir), GARAGE_CONFIG_FILE=str(self.config))
        self.compose = ["docker", "compose", "--env-file", "/dev/null", "-f",
                        str(compose_file or ROOT / ("docker-compose.prod.yml" if mode == "prod" else "docker-compose.yml"))]

    def run(self, args, *, check=True, capture=False):
        result = subprocess.run(self.compose + list(args), cwd=ROOT, env=self.env,
                                text=True, stdout=subprocess.PIPE if capture else None,
                                stderr=subprocess.PIPE if capture else None)
        if check and result.returncode:
            # Captured CLI output may contain secret values. Never echo it.
            raise RuntimeError("Compose/Garage operation failed; inspect service logs locally")
        return result

    def garage(self, *args, check=True):
        return self.run(["exec", "-T", "storage", "/garage", *args], check=check, capture=True)

    def bootstrap(self):
        for _ in range(60):
            if self.garage("status", check=False).returncode == 0:
                break
            time.sleep(2)
        else:
            raise RuntimeError("Garage RPC did not become ready within 120 seconds")
        layout = self.garage("layout", "show").stdout
        version = re.search(r"Current cluster layout version:\s*(\d+)", layout)
        if not version:
            raise RuntimeError("Unrecognized Garage v2.2 layout output; refusing to change layout")
        if int(version.group(1)) == 0:
            node = self.garage("node", "id", "-q").stdout.strip().split("@", 1)[0]
            if not re.fullmatch(r"[0-9a-f]{64}", node):
                raise RuntimeError("Invalid Garage node ID")
            self.garage("layout", "assign", "-z", os.environ.get("GARAGE_LAYOUT_ZONE", "dc1"),
                        "-c", os.environ.get("GARAGE_LAYOUT_CAPACITY", "1G"), node)
            self.garage("layout", "apply", "--version", "1")
            if not re.search(r"Current cluster layout version:\s*1\b", self.garage("layout", "show").stdout):
                raise RuntimeError("Garage layout was not applied")
        bucket = os.environ.get("S3_BUCKET", "klubhub")
        name = os.environ.get("S3_ACCESS_KEY_NAME", "klubhub-app-key")
        if self.garage("bucket", "info", bucket, check=False).returncode:
            self.garage("bucket", "create", bucket)
        self.garage("bucket", "info", bucket)
        access_file = self.secret_dir / "s3_access_key"
        secret_file = self.secret_dir / "s3_secret_key"
        # Resume after interruption: the existing key's secret is recoverable by
        # authenticated RPC. A duplicate/ambiguous key name fails closed.
        for path in (access_file, secret_file):
            if path.is_symlink():
                raise RuntimeError("Refusing symlink credential file")
        selector = access_file.read_text().strip() if access_file.exists() else name
        result = self.garage("key", "info", "--show-secret", selector, check=False)
        if result.returncode:
            if access_file.exists() or secret_file.exists():
                raise RuntimeError("Existing S3 credentials do not match this cluster; refusing replacement")
            # Check exact key names before create: Garage permits duplicate names.
            listing = self.garage("key", "list").stdout
            if name in listing:
                raise RuntimeError("Key lookup failed but name exists; refusing duplicate creation")
            result = self.garage("key", "create", name)
        access = re.search(r"^Key ID:\s*(GK[0-9a-f]+)\s*$", result.stdout, re.M)
        secret = re.search(r"^Secret key:\s*([0-9a-f]+)\s*$", result.stdout, re.M)
        if not access or not secret:
            raise RuntimeError("Unrecognized Garage key output (credentials withheld)")
        for path, value in ((access_file, access.group(1)), (secret_file, secret.group(1))):
            if path.is_symlink():
                raise RuntimeError("Refusing symlink credential file")
            if path.exists() and path.read_text().strip() != value:
                raise RuntimeError("Existing S3 credential differs; refusing replacement")
            publish(path, value + "\n")
        self.garage("bucket", "allow", bucket, "--read", "--write", "--key", access.group(1))
        self.garage("key", "info", access.group(1))
        info = self.garage("bucket", "info", bucket).stdout
        if not re.search(r"^RW\S*\s+" + re.escape(access.group(1)) + r"\b", info, re.M):
            raise RuntimeError("Garage bucket permissions could not be verified")
        print("Garage provisioned; S3 credentials saved privately (not printed).")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("mode", nargs="?", choices=("dev", "prod"), default=None)
    parser.add_argument("-f", "--file", type=Path, help="Compose file (prod filename implies prod defaults)")
    group = parser.add_mutually_exclusive_group()
    group.add_argument("--prepare-only", action="store_true", help="Create missing private files only; no Docker")
    group.add_argument("--bootstrap-only", action="store_true", help="Provision already-running storage only")
    args = parser.parse_args()
    mode = args.mode or ("prod" if args.file and "prod" in args.file.name else "dev")
    stack = Stack(mode, args.file.absolute() if args.file else None)
    os.umask(0o077)
    private_dir(stack.secret_dir)
    lock_path = stack.secret_dir / ".setup.lock"
    fd = os.open(lock_path, os.O_CREAT | os.O_RDWR | os.O_NOFOLLOW, 0o600)
    with os.fdopen(fd, "w") as lock:
        fcntl.flock(lock, fcntl.LOCK_EX)
        if not args.bootstrap_only:
            prepare(stack.secret_dir, stack.config)
        if args.prepare_only:
            print("Private setup files ready; existing files preserved.")
            return
        if not args.bootstrap_only:
            stack.run(["up", "-d", "--wait", "storage"])
        stack.bootstrap()
        if not args.bootstrap_only:
            stack.run(["up", "-d", "--wait"] + (["--build"] if mode == "dev" else []))
            print("Stack ready.")
            if mode == "dev":
                print("Host frontend: NUXT_PUBLIC_API_BASE=http://127.0.0.1:8080 pnpm nx serve @dev/dj --host 0.0.0.0 --port 4200")


if __name__ == "__main__":
    try:
        main()
    except (RuntimeError, OSError) as exc:
        print(f"Setup failed: {exc}", file=sys.stderr)
        sys.exit(1)
