#!/usr/bin/env python3
"""Hermetic tests for scripts/install.sh (the one-line installer).

Nothing here installs anything: install.sh is *sourced* (its main() only runs
when executed), every external command that could touch the machine (docker,
curl, aws, the restore/backup scripts) is a stub on PATH, and every directory
is a fresh temp dir under TMPDIR. ~/klubhub is never read or written (HOME is
redirected for each run).

Run: TMPDIR="$HOME/.hermes/cache/scratch" python3 scripts/tests/test_install.py
"""
import json
import os
from pathlib import Path
import pty
import shutil
import subprocess
import tempfile
import unittest

SCRIPTS = Path(__file__).resolve().parents[1]
INSTALL = SCRIPTS / 'install.sh'


class Sandbox:
    """A temp HOME/DIR plus a stub bin directory, cleaned up afterwards."""

    def __init__(self, test):
        self.root = Path(tempfile.mkdtemp(prefix='install-test-'))
        test.addCleanup(shutil.rmtree, self.root, ignore_errors=True)
        self.home = self.root / 'home'
        self.home.mkdir()
        self.dir = self.root / 'inst'
        self.bin = self.root / 'bin'
        self.bin.mkdir()
        # Safety net: anything not explicitly stubbed by a test must fail, never
        # reach the real docker daemon, network or repository.
        for name in ('docker', 'git', 'curl', 'aws'):
            self.stub(name, f'echo "unstubbed {name} $*" >&2; exit 99\n')

    def stub(self, name, body):
        path = self.bin / name
        path.write_text('#!/usr/bin/env bash\n' + body)
        path.chmod(0o755)

    def env(self, **extra):
        env = {k: v for k, v in os.environ.items() if not k.startswith('KLUBHUB_')}
        env.update(HOME=str(self.home), TMPDIR=str(self.root),
                   PATH=f'{self.bin}:{env["PATH"]}')
        env.update(extra)
        return env

    def bash(self, script, **extra):
        # New session + closed stdin: no controlling terminal, so /dev/tty is
        # never read and the installer can never prompt the developer.
        return subprocess.run(['bash', '-c', script], env=self.env(**extra), stdin=subprocess.DEVNULL,
                              capture_output=True, text=True, timeout=60, start_new_session=True)


def sourced(body):
    return f'source "{INSTALL}"\n{body}'


class SourceSafetyTests(unittest.TestCase):
    def test_sourcing_defines_functions_without_running_main(self):
        sb = Sandbox(self)
        r = sb.bash(sourced('type parse_args >/dev/null && echo SOURCED_OK'))
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertIn('SOURCED_OK', r.stdout)
        self.assertNotIn('KlubHub installer', r.stdout)

    def test_help_when_executed(self):
        r = subprocess.run(['bash', str(INSTALL), '--help'], capture_output=True, text=True, timeout=30)
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertIn('Usage: install.sh', r.stdout)

    def test_help_when_piped_like_curl_bash(self):
        r = subprocess.run(['bash', '-s', '--', '--help'], input=INSTALL.read_text(),
                           capture_output=True, text=True, timeout=30)
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertIn('Usage: install.sh', r.stdout)


def dj_set(sb, *args, **env):
    """Run parse_args and return (returncode, DJ_SET lines, stderr)."""
    quoted = ' '.join("'" + a.replace("'", "'\\''") + "'" for a in args)
    r = sb.bash(sourced(f'parse_args {quoted}\nfor kv in "${{DJ_SET[@]}}"; do echo "$kv"; done'), **env)
    return r.returncode, [x for x in r.stdout.splitlines() if x], r.stderr


class DjUrlNormalizationTests(unittest.TestCase):
    ACCEPT = {
        'https://dj.example': 'https://dj.example',
        'https://dj.example/': 'https://dj.example',
        'HTTPS://DJ.Example:8443/': 'https://dj.example:8443',
        'http://192.168.1.5:8080': 'http://192.168.1.5:8080',
        'http://[::1]:8080/': 'http://[::1]:8080',
        'https://dj.example:443': 'https://dj.example',
        'http://dj.example:80/': 'http://dj.example',
        'http://localhost:3000': 'http://localhost:3000',
    }
    REJECT = [
        'https://dj.example/app', 'https://dj.example//', 'https://dj.example/?a=1',
        'https://dj.example?a=1', 'https://dj.example#frag', 'https://dj.example/#frag',
        'https://user:pw@dj.example', 'https://user@dj.example', 'ftp://dj.example',
        'dj.example', 'https://', 'https:///', 'https://dj.example:0',
        'https://dj.example:70000', 'https://dj.example:abc', 'https://dj .example',
        'https://dj.example\\', 'http://:8080',
    ]

    def test_accepted_urls_normalize_to_exact_origin(self):
        sb = Sandbox(self)
        for raw, want in self.ACCEPT.items():
            with self.subTest(raw=raw):
                rc, lines, err = dj_set(sb, '--dj-url', raw)
                self.assertEqual(rc, 0, err)
                self.assertEqual(lines, [f'CORS_ORIGIN={want}'])

    def test_rejected_urls_fail_with_clear_error(self):
        sb = Sandbox(self)
        for raw in self.REJECT:
            with self.subTest(raw=raw):
                rc, lines, err = dj_set(sb, '--dj-url', raw)
                self.assertNotEqual(rc, 0)
                self.assertEqual(lines, [])
                self.assertIn('--dj-url', err)

    def test_error_never_echoes_credentials(self):
        sb = Sandbox(self)
        rc, _, err = dj_set(sb, '--dj-url', 'https://admin:hunter2@dj.example')
        self.assertNotEqual(rc, 0)
        self.assertNotIn('hunter2', err)
        self.assertIn('credentials', err)

    def test_env_var_is_normalized_and_validated(self):
        sb = Sandbox(self)
        rc, lines, err = dj_set(sb, KLUBHUB_DJ_URL='https://DJ.example/')
        self.assertEqual((rc, lines), (0, ['CORS_ORIGIN=https://dj.example']), err)
        rc, lines, err = dj_set(sb, KLUBHUB_DJ_URL='https://dj.example/path')
        self.assertNotEqual(rc, 0)
        self.assertIn('KLUBHUB_DJ_URL', err)

    def test_dj_env_cors_origin_cannot_bypass_validation(self):
        sb = Sandbox(self)
        rc, lines, err = dj_set(sb, '--dj-env', 'CORS_ORIGIN=https://dj.example/')
        self.assertEqual((rc, lines), (0, ['CORS_ORIGIN=https://dj.example']), err)
        rc, _, err = dj_set(sb, '--dj-env', 'CORS_ORIGIN=https://dj.example/x')
        self.assertNotEqual(rc, 0)

    def test_s3_url_and_promoter_origin_semantics_unchanged(self):
        # Checked against their consumers: the API only uses scheme+host of
        # S3_PUBLIC_ENDPOINT, and Promoter normalizes PROMOTER_PUBLIC_ORIGIN to
        # scheme://host itself, so neither needs the strict DJ rule.
        sb = Sandbox(self)
        rc, lines, err = dj_set(sb, '--s3-url', 'https://files.example/')
        self.assertEqual((rc, lines), (0, ['S3_PUBLIC_ENDPOINT=https://files.example/']), err)
        r = sb.bash(sourced("parse_args --origin https://p.example/x\necho \"$ORIGIN\""))
        self.assertEqual(r.stdout.strip(), 'https://p.example/x', r.stderr)


def seed_dj_env(sb, **values):
    local = sb.dir / '.local'
    local.mkdir(parents=True, exist_ok=True)
    (local / 'dj.env').write_text(''.join(f'{k}={v}\n' for k, v in values.items()))
    return local / 'dj.env'


def read_env(path):
    return dict(line.split('=', 1) for line in path.read_text().splitlines() if '=' in line)


class ExposureWarningTests(unittest.TestCase):
    def warning(self, sb, **env):
        seed_dj_env(sb, **env)
        # DJ_PORT is set by load_settings in the real flow; the helper needs it under set -u.
        r = sb.bash(sourced(f'DIR="{sb.dir}"; DJ_PORT=8080\ncors_exposure_warning'))
        self.assertEqual(r.returncode, 0, r.stderr)
        return r.stdout + r.stderr

    def test_warns_when_published_beyond_loopback_without_public_url(self):
        sb = Sandbox(self)
        for env in ({'API_BIND': '0.0.0.0'},
                    {'API_BIND': '192.168.1.5', 'CORS_ORIGIN': 'http://127.0.0.1:8080'},
                    {'API_BIND': '0.0.0.0', 'CORS_ORIGIN': 'http://localhost:8080'}):
            with self.subTest(env=env):
                out = self.warning(sb, **env)
                self.assertIn('403', out)
                self.assertIn('--dj-url', out)

    def test_silent_when_loopback_or_public_url_set(self):
        sb = Sandbox(self)
        for env in ({}, {'API_BIND': '127.0.0.1'},
                    {'API_BIND': '0.0.0.0', 'CORS_ORIGIN': 'https://dj.example'},
                    {'API_BIND': '0.0.0.0', 'CORS_ORIGIN': 'http://192.168.1.5:8080'}):
            with self.subTest(env=env):
                self.assertEqual(self.warning(sb, **env), '')

    def test_rule_lines_show_effective_origin_and_allowed_hosts(self):
        sb = Sandbox(self)
        seed_dj_env(sb, CORS_ORIGIN='https://dj.example')
        r = sb.bash(sourced(f'DIR="{sb.dir}"\ncors_rule_lines'))
        self.assertEqual(r.returncode, 0, r.stderr)
        for want in ('CORS_ORIGIN=https://dj.example', 'localhost', '127.0.0.1', '[::1]', 'dj.example', 'any port'):
            self.assertIn(want, r.stdout)

    def test_rule_lines_without_public_host_list_loopback_only(self):
        sb = Sandbox(self)
        seed_dj_env(sb, CORS_ORIGIN='http://127.0.0.1:8080')
        r = sb.bash(sourced(f'DIR="{sb.dir}"\ncors_rule_lines'))
        self.assertIn('loopback only', r.stdout)


class OfferPublicUrlTests(unittest.TestCase):
    def run_offer(self, sb, answers, yes='0', bind='0.0.0.0', **env):
        envfile = seed_dj_env(sb, API_BIND=bind, CORS_ORIGIN='http://127.0.0.1:8080', **env)
        master, slave = pty.openpty()
        self.addCleanup(os.close, master)
        self.addCleanup(os.close, slave)
        os.write(master, answers.encode())
        r = sb.bash(sourced(f'DIR="{sb.dir}"; YES={yes}; CHECK=0; DJ=1; TTY="{os.ttyname(slave)}"\noffer_public_url'))
        self.assertEqual(r.returncode, 0, r.stderr)
        return read_env(envfile), r

    def test_interactive_prompt_stores_normalized_public_url(self):
        sb = Sandbox(self)
        env, _ = self.run_offer(sb, 'https://DJ.example/\n')
        self.assertEqual(env['CORS_ORIGIN'], 'https://dj.example')

    def test_invalid_answer_is_reprompted_not_stored(self):
        sb = Sandbox(self)
        env, r = self.run_offer(sb, 'https://dj.example/app\nhttps://dj.example\n')
        self.assertEqual(env['CORS_ORIGIN'], 'https://dj.example')
        self.assertIn('origin only', r.stderr)

    def test_blank_answer_keeps_default_and_never_invents_a_hostname(self):
        sb = Sandbox(self)
        env, _ = self.run_offer(sb, '\n')
        self.assertEqual(env['CORS_ORIGIN'], 'http://127.0.0.1:8080')

    def test_yes_never_prompts_and_never_changes_origin(self):
        sb = Sandbox(self)
        env, r = self.run_offer(sb, 'https://dj.example\n', yes='1')
        self.assertEqual(env['CORS_ORIGIN'], 'http://127.0.0.1:8080')

    def test_no_prompt_when_bound_to_loopback_or_url_already_custom(self):
        sb = Sandbox(self)
        env, _ = self.run_offer(sb, 'https://dj.example\n', bind='127.0.0.1')
        self.assertEqual(env['CORS_ORIGIN'], 'http://127.0.0.1:8080')
        sb2 = Sandbox(self)
        envfile = seed_dj_env(sb2, API_BIND='0.0.0.0', CORS_ORIGIN='https://mine.example')
        master, slave = pty.openpty()
        self.addCleanup(os.close, master)
        self.addCleanup(os.close, slave)
        os.write(master, b'https://other.example\n')
        r = sb2.bash(sourced(f'DIR="{sb2.dir}"; YES=0; CHECK=0; DJ=1; TTY="{os.ttyname(slave)}"\noffer_public_url'))
        self.assertEqual(read_env(envfile)['CORS_ORIGIN'], 'https://mine.example', r.stderr)


class MainWiringTests(unittest.TestCase):
    """main/summary/check_only must actually use the CORS helpers: without the
    calls a LAN-published DJ silently 403s every save with no guidance."""
    STUBS = (
        'parse_args(){ :; }; choose_products(){ :; }; preflight(){ :; }; fetch_repo(){ :; }\n'
        'write_settings(){ :; }; install_promoter(){ echo install_promoter; }\n'
        'load_settings(){ echo load_settings; }; offer_public_url(){ echo offer_public_url; }\n'
        'cors_exposure_warning(){ echo cors_exposure_warning; }; check_only(){ echo check_only; }\n'
        'install_dj(){ echo install_dj; }; summary(){ echo summary; }\n'
    )

    def flow(self, dj, check):
        sb = Sandbox(self)
        r = sb.bash(sourced(f'DIR="{sb.dir}"\n{self.STUBS}DJ={dj}; PROMOTER=0; CHECK={check}\nmain'))
        self.assertEqual(r.returncode, 0, r.stderr)
        return [line for line in r.stdout.splitlines() if line.replace('_', '').isalpha()]

    def test_dj_install_offers_url_and_warns_after_settings_and_before_install(self):
        self.assertEqual(self.flow(1, 0), ['load_settings', 'offer_public_url', 'cors_exposure_warning', 'install_dj', 'summary'])

    def test_check_mode_warns_before_returning_without_installing(self):
        self.assertEqual(self.flow(1, 1), ['load_settings', 'offer_public_url', 'cors_exposure_warning', 'check_only'])

    def test_promoter_only_run_never_touches_dj_cors_guidance(self):
        self.assertEqual(self.flow(0, 0), ['load_settings', 'summary'])

    def test_summary_and_check_only_list_the_cors_rules_for_dj(self):
        sb = Sandbox(self)
        seed_dj_env(sb, CORS_ORIGIN='https://dj.example', API_BIND='0.0.0.0')
        for fn in ('summary', 'check_only'):
            with self.subTest(fn=fn):
                r = sb.bash(sourced(
                    f'DIR="{sb.dir}"; DJ=1; PROMOTER=0; DJ_PORT=8080; S3_PORT_V=39000\n'
                    'resolve_tag(){ :; }; check_ports(){ :; }\n' + fn))
                self.assertEqual(r.returncode, 0, r.stderr)
                self.assertIn('CORS_ORIGIN=https://dj.example', r.stdout)
                self.assertIn('Allowed Host for saves', r.stdout)


class LoadSettingsUpdateTests(unittest.TestCase):
    def update(self, sb, *args, **seed):
        envfile = seed_dj_env(sb, **seed)
        quoted = ' '.join(args)
        r = sb.bash(sourced(f'parse_args --dir "{sb.dir}" --dj {quoted}\nload_settings'))
        self.assertEqual(r.returncode, 0, r.stderr)
        return read_env(envfile), r

    def test_default_local_origin_follows_a_port_change(self):
        env, _ = self.update(Sandbox(self), '--dj-port 9090', API_PORT='8080', CORS_ORIGIN='http://127.0.0.1:8080')
        self.assertEqual((env['API_PORT'], env['CORS_ORIGIN']), ('9090', 'http://127.0.0.1:9090'))

    def test_custom_origin_survives_port_change_and_update(self):
        for custom in ('https://dj.example', 'http://192.168.1.5:8080', 'http://localhost:8080'):
            with self.subTest(custom=custom):
                env, _ = self.update(Sandbox(self), '--dj-port 9090', API_PORT='8080', CORS_ORIGIN=custom)
                self.assertEqual(env['CORS_ORIGIN'], custom)
                env, _ = self.update(Sandbox(self), CORS_ORIGIN=custom)
                self.assertEqual(env['CORS_ORIGIN'], custom)

    def test_explicit_dj_url_wins_over_saved_value(self):
        env, _ = self.update(Sandbox(self), '--dj-url https://new.example/', API_PORT='8080', CORS_ORIGIN='https://old.example')
        self.assertEqual(env['CORS_ORIGIN'], 'https://new.example')

    def test_warns_when_custom_origin_still_names_the_old_port(self):
        _, r = self.update(Sandbox(self), '--dj-port 9090', API_PORT='8080', CORS_ORIGIN='http://192.168.1.5:8080')
        self.assertIn('9090', r.stderr)
        self.assertIn('--dj-url', r.stderr)

    def test_no_port_warning_for_proxy_origin_without_port(self):
        _, r = self.update(Sandbox(self), '--dj-port 9090', API_PORT='8080', CORS_ORIGIN='https://dj.example')
        self.assertNotIn('9090', r.stderr)

    def test_saved_origin_from_older_installer_is_normalized(self):
        env, r = self.update(Sandbox(self), CORS_ORIGIN='https://DJ.example/')
        self.assertEqual(env['CORS_ORIGIN'], 'https://dj.example')
        self.assertIn('normalized', r.stderr)

    def test_unfixable_saved_origin_is_left_alone_with_a_warning(self):
        env, r = self.update(Sandbox(self), CORS_ORIGIN='https://dj.example/app')
        self.assertEqual(env['CORS_ORIGIN'], 'https://dj.example/app')
        self.assertIn('--dj-url', r.stderr)


if __name__ == '__main__':
    unittest.main()
