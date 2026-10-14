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
import tarfile
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
        self.new_session = True
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
                              capture_output=True, text=True, timeout=60, start_new_session=self.new_session)


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
        # No new session: a session leader that opens the pty slave acquires it as
        # controlling terminal and then blocks on exit while the master is open
        # (macOS). The offer flow reads $TTY (the slave), never /dev/tty.
        sb.new_session = False
        # DJ_PORT is set by load_settings in the real flow; the helper needs it under set -u.
        r = sb.bash(sourced(f'DIR="{sb.dir}"; DJ_PORT=8080; YES={yes}; CHECK=0; DJ=1; TTY="{os.ttyname(slave)}"\noffer_public_url'))
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


LS_REMOTE_OUT = '''aaa111	refs/tags/v1.10.0
bbb222	refs/tags/v1.10.0^{}
ccc333	refs/tags/v1.9.0
ddd444	refs/tags/v0.1.0-rc1
'''


def stub_ls_remote(sb, output=LS_REMOTE_OUT):
    """git stub that answers ls-remote from a file; everything else fails."""
    out = sb.root / 'ls-remote.txt'
    out.write_text(output)
    sb.stub('git', f'''if [[ "$1" == ls-remote ]]; then cat "{out}"; exit 0; fi
echo "unstubbed git $*" >&2; exit 99
''')


class LatestReleaseTests(unittest.TestCase):
    def test_picks_newest_v_tag_and_skips_annotated_deref_lines(self):
        sb = Sandbox(self)
        stub_ls_remote(sb)
        r = sb.bash(sourced('latest_release'))
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertEqual(r.stdout.strip(), 'v1.10.0')


class ResolveRefTests(unittest.TestCase):
    def resolve(self, sb, *args, seed=None):
        if seed:
            local = sb.dir / '.local'
            local.mkdir(parents=True, exist_ok=True)
            (local / 'installer.env').write_text(seed)
        stub_ls_remote(sb)
        quoted = ' '.join(args)
        return sb.bash(sourced(f'''parse_args --dir "{sb.dir}" {quoted}
resolve_ref
echo "REF=$REF"'''))

    def test_release_flag_picks_newest_v_tag(self):
        r = self.resolve(Sandbox(self), '--release')
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertIn('REF=v1.10.0', r.stdout)
        self.assertIn('latest release', r.stdout)

    def test_explicit_ref_wins_over_release(self):
        r = self.resolve(Sandbox(self), '--release', '--ref', 'v1.9.0')
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertIn('REF=v1.9.0', r.stdout)

    def test_saved_ref_is_used_without_flags(self):
        r = self.resolve(Sandbox(self), seed='''KLUBHUB_REF=v1.1.0
''')
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertIn('REF=v1.1.0', r.stdout)

    def test_release_overrides_saved_main(self):
        r = self.resolve(Sandbox(self), '--release', seed='''KLUBHUB_REF=main
''')
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertIn('REF=v1.10.0', r.stdout)

    def test_no_flags_defaults_to_main(self):
        r = self.resolve(Sandbox(self))
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertIn('REF=main', r.stdout)

    def test_release_without_any_v_tag_dies(self):
        sb = Sandbox(self)
        stub_ls_remote(sb, output='')
        r = sb.bash(sourced(f'''parse_args --dir "{sb.dir}" --release
resolve_ref'''))
        self.assertNotEqual(r.returncode, 0)
        self.assertIn('No v* release tags', r.stderr)

    def test_backup_only_flag_parses(self):
        sb = Sandbox(self)
        r = sb.bash(sourced(f'''parse_args --dir "{sb.dir}" --backup-only
echo "BACKUP_ONLY=$BACKUP_ONLY"'''))
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertIn('BACKUP_ONLY=1', r.stdout)


class WriteSettingsTests(unittest.TestCase):
    PREV = '''KLUBHUB_REF=main
KLUBHUB_COMMIT=aaa111bbb
KLUBHUB_DJ_TAG=sha-old123
KLUBHUB_PROMOTER_TAG=sha-old456
'''

    def run_write(self, sb, seed):
        local = sb.dir / '.local'
        local.mkdir(parents=True, exist_ok=True)
        if seed:
            (local / 'installer.env').write_text(seed)
        r = sb.bash(sourced(f'''DIR="{sb.dir}"
DJ_KEEP=0; PROMOTER_KEEP=0; DJ=1; PROMOTER=1
REF=v1.2.0; COMMIT_FULL=bbb222ccc
DJ_TAG=sha-new123; PROMOTER_TAG=sha-new456
ORIGIN=http://localhost:8090; INSTALLER_URL=https://example.invalid/install.sh
write_settings'''))
        self.assertEqual(r.returncode, 0, r.stderr)
        return read_env(local / 'installer.env'), (sb.dir / 'klubhub').read_text()

    def test_update_records_previous_version_for_rollback(self):
        env, wrapper = self.run_write(Sandbox(self), self.PREV)
        self.assertEqual(env['KLUBHUB_REF'], 'v1.2.0')
        self.assertEqual(env['KLUBHUB_COMMIT'], 'bbb222ccc')
        self.assertEqual(env['KLUBHUB_DJ_TAG'], 'sha-new123')
        self.assertEqual(env['KLUBHUB_PROMOTER_TAG'], 'sha-new456')
        self.assertEqual(env['KLUBHUB_PREV_REF'], 'main')
        self.assertEqual(env['KLUBHUB_PREV_COMMIT'], 'aaa111bbb')
        self.assertEqual(env['KLUBHUB_PREV_DJ_TAG'], 'sha-old123')
        self.assertEqual(env['KLUBHUB_PREV_PROMOTER_TAG'], 'sha-old456')
        for needle in ('rollback)', 'backup)', '--release', 'KLUBHUB_PREV_COMMIT'):
            self.assertIn(needle, wrapper)

    def test_first_install_leaves_previous_version_empty(self):
        env, _ = self.run_write(Sandbox(self), None)
        self.assertEqual(env['KLUBHUB_PREV_REF'], '')
        self.assertEqual(env['KLUBHUB_PREV_COMMIT'], '')
        self.assertEqual(env['KLUBHUB_PREV_DJ_TAG'], '')
        self.assertEqual(env['KLUBHUB_PREV_PROMOTER_TAG'], '')

    def test_generated_wrapper_is_valid_bash(self):
        sb = Sandbox(self)
        self.run_write(sb, self.PREV)
        r = subprocess.run(['bash', '-n', str(sb.dir / 'klubhub')], capture_output=True, text=True, timeout=30)
        self.assertEqual(r.returncode, 0, r.stderr)

    def test_generated_wrapper_rollback_without_previous_version_fails_cleanly(self):
        sb = Sandbox(self)
        self.run_write(sb, None)
        r = subprocess.run(['bash', str(sb.dir / 'klubhub'), 'rollback'],
                           capture_output=True, text=True, timeout=30)
        self.assertNotEqual(r.returncode, 0)
        self.assertIn('No previous version recorded', r.stderr)


class BackupStackTests(unittest.TestCase):
    DOCKER_STUB = '''case "$*" in
  *"ps -q db"*|*"ps -q promoter-db"*) [[ "${SB_RUNNING:-0}" == 1 ]] && echo fake-container-id; exit 0 ;;
  *"exec -T db pg_dump"*) echo "-- DJ dump"; exit 0 ;;
  *"exec -T promoter-db pg_dump"*) echo "-- Promoter dump"; exit 0 ;;
  *"cp db:/var/lib/postgresql/data/."*|*"cp promoter-db:/var/lib/postgresql/data/."*)
    dest="${@: -1}"; mkdir -p "$dest"; echo raw > "$dest/PG_VERSION"; exit 0 ;;
  *"cp storage:/data/."*)
    dest="${@: -1}"; mkdir -p "$dest"; echo obj > "$dest/object"; exit 0 ;;
  *"image prune"*) echo pruned; exit 0 ;;
  *) echo "unstubbed docker $*" >&2; exit 99 ;;
esac
'''

    def seed_install(self, sb):
        local = sb.dir / '.local'
        local.mkdir(parents=True, exist_ok=True)
        (local / 'installer.env').write_text('''KLUBHUB_DJ=1
KLUBHUB_PROMOTER=1
KLUBHUB_DJ_TAG=sha-old123
KLUBHUB_PROMOTER_TAG=sha-old456
''')
        (local / 'dj.env').write_text('''API_PORT=8080
''')
        (local / 'promoter.env').write_text('''PROMOTER_HOST_PORT=8090
''')
        secrets = sb.dir / 'secrets' / 'prod'
        secrets.mkdir(parents=True)
        (secrets / 'postgres_password').write_text('''s3cret
''')
        prom = local / 'promoter'
        prom.mkdir()
        (prom / 'kek').write_text('''kek-material
''')

    def run_backup(self, sb, running):
        self.seed_install(sb)
        sb.stub('docker', self.DOCKER_STUB)
        r = sb.bash(sourced(f'''DIR="{sb.dir}"; DJ=1; PROMOTER=1
ORIGIN=http://localhost:8090; PROMOTER_TAG=sha-y
backup_stack
echo "BACKUP_DIR=$BACKUP_DIR"'''), SB_RUNNING=running)
        self.assertEqual(r.returncode, 0, r.stderr)
        return r

    @staticmethod
    def backup_dir(r):
        return Path(r.stdout.strip().splitlines()[-1].split('=', 1)[1])

    def test_running_stack_is_dumped_logically(self):
        sb = Sandbox(self)
        r = self.run_backup(sb, '1')
        backup = self.backup_dir(r)
        self.assertTrue(str(backup).startswith(str(sb.dir / '.local' / 'backups')))
        self.assertEqual((backup / 'dj-postgres.sql').read_text().strip(), '-- DJ dump')
        self.assertEqual((backup / 'promoter-postgres.sql').read_text().strip(), '-- Promoter dump')
        self.assertFalse((backup / 'dj-postgres-data.tar.gz').exists())
        self.assertEqual((backup / 'secrets-prod' / 'postgres_password').read_text().strip(), 's3cret')
        self.assertEqual((backup / 'promoter-local' / 'kek').read_text().strip(), 'kek-material')
        for f in ('dj.env', 'promoter.env', 'installer.env'):
            self.assertTrue((backup / f).exists(), f)
        with tarfile.open(backup / 'dj-garage-data.tar.gz') as tf:
            self.assertIn('data/object', tf.getnames())
        self.assertIn('backup complete', r.stdout)

    def test_stopped_stack_is_copied_raw(self):
        sb = Sandbox(self)
        r = self.run_backup(sb, '0')
        backup = self.backup_dir(r)
        self.assertFalse((backup / 'dj-postgres.sql').exists())
        self.assertFalse((backup / 'promoter-postgres.sql').exists())
        for name in ('dj-postgres-data.tar.gz', 'promoter-postgres-data.tar.gz'):
            with tarfile.open(backup / name) as tf:
                self.assertIn('data/PG_VERSION', tf.getnames())


class HealthGateTests(unittest.TestCase):
    def test_wait_http_returns_1_on_timeout_instead_of_dying(self):
        sb = Sandbox(self)  # the default curl stub always fails
        r = sb.bash(sourced(f'''DIR="{sb.dir}"
wait_http "http://127.0.0.1:9/api/v1/health" "DJ" 0 || echo "rc=$?"'''))
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertIn('rc=1', r.stdout)

    def test_health_failed_points_at_rollback_after_an_update(self):
        sb = Sandbox(self)
        r = sb.bash(sourced(f'''DIR="{sb.dir}"; IS_UPDATE=1
health_failed "DJ"'''))
        self.assertNotEqual(r.returncode, 0)
        self.assertIn('klubhub rollback', r.stderr)
        self.assertIn('KLUBHUB_PREV_', r.stderr)

    def test_health_failed_without_update_keeps_plain_message(self):
        sb = Sandbox(self)
        r = sb.bash(sourced(f'''DIR="{sb.dir}"; IS_UPDATE=0
health_failed "DJ"'''))
        self.assertNotEqual(r.returncode, 0)
        self.assertNotIn('rollback', r.stderr)
        self.assertIn('did not become ready', r.stderr)


class MainUpdateWiringTests(unittest.TestCase):
    """An update (installer.env exists) must back up before installing and
    prune dangling images after; --backup-only must only back up."""
    STUBS = '''parse_args(){ :; }; choose_products(){ :; }
preflight(){ echo preflight; }; fetch_repo(){ echo fetch_repo; }
write_settings(){ :; }; install_promoter(){ echo install_promoter; }
load_settings(){ echo load_settings; }; offer_public_url(){ echo offer_public_url; }
cors_exposure_warning(){ echo cors_exposure_warning; }; check_only(){ echo check_only; }
install_dj(){ echo install_dj; }; summary(){ echo summary; }
backup_stack(){ echo backup_stack; }; prune_images(){ echo prune_images; }
'''

    def flow(self, sb, prelude):
        r = sb.bash(sourced(f'''DIR="{sb.dir}"
{self.STUBS}{prelude}
main'''))
        self.assertEqual(r.returncode, 0, r.stderr)
        return [line for line in r.stdout.splitlines() if line.replace('_', '').isalpha()]

    def seed_installer_env(self, sb):
        local = sb.dir / '.local'
        local.mkdir(parents=True, exist_ok=True)
        (local / 'installer.env').write_text('''KLUBHUB_DJ=1
KLUBHUB_PROMOTER=0
''')

    def test_update_backs_up_before_install_and_prunes_after(self):
        sb = Sandbox(self)
        self.seed_installer_env(sb)
        flow = self.flow(sb, 'DJ=1; PROMOTER=0; CHECK=0')
        self.assertEqual(flow, ['preflight', 'fetch_repo', 'load_settings', 'offer_public_url',
                                'cors_exposure_warning', 'backup_stack', 'install_dj',
                                'prune_images', 'summary'])

    def test_backup_only_never_touches_preflight_or_fetch(self):
        sb = Sandbox(self)
        self.seed_installer_env(sb)
        flow = self.flow(sb, 'DJ=1; PROMOTER=0; CHECK=0; BACKUP_ONLY=1')
        self.assertEqual(flow, ['load_settings', 'backup_stack'])


class WrapperCommandTests(unittest.TestCase):
    """The generated ./klubhub must translate subcommands into the right
    installer invocations (update --release, backup, rollback flags)."""

    def make_wrapper(self, sb):
        local = sb.dir / '.local'
        local.mkdir(parents=True, exist_ok=True)
        # Pre-update state: write_settings records these as the rollback target.
        (local / 'installer.env').write_text('''KLUBHUB_DJ=1
KLUBHUB_PROMOTER=1
KLUBHUB_REF=main
KLUBHUB_COMMIT=aaa111
KLUBHUB_DJ_TAG=sha-old123
KLUBHUB_PROMOTER_TAG=sha-old456
KLUBHUB_ORIGIN=http://localhost:8090
KLUBHUB_INSTALLER_URL=https://example.invalid/install.sh
''')
        r = sb.bash(sourced(f'''DIR="{sb.dir}"
DJ_KEEP=0; PROMOTER_KEEP=0; DJ=1; PROMOTER=1
REF=v1.2.0; COMMIT_FULL=bbb222
DJ_TAG=sha-new123; PROMOTER_TAG=sha-new456
ORIGIN=http://localhost:8090; INSTALLER_URL=https://example.invalid/install.sh
write_settings'''))
        self.assertEqual(r.returncode, 0, r.stderr)
        # Replace the saved installer with a stub that records its arguments.
        stub = local / 'install.sh'
        stub.write_text('''#!/usr/bin/env bash
echo "$*"
''')
        stub.chmod(0o755)

    def run_wrapper(self, sb, *args):
        return subprocess.run(['bash', str(sb.dir / 'klubhub'), *args],
                              capture_output=True, text=True, timeout=30)

    def test_update_without_args_uses_saved_ref(self):
        sb = Sandbox(self)
        self.make_wrapper(sb)
        r = self.run_wrapper(sb, 'update')
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertEqual(r.stdout.strip(), f'--dj --promoter --dir {sb.dir} --ref v1.2.0 --yes')

    def test_update_release_invokes_installer_with_release_flag(self):
        sb = Sandbox(self)
        self.make_wrapper(sb)
        r = self.run_wrapper(sb, 'update', '--release')
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertEqual(r.stdout.strip(), f'--dj --promoter --dir {sb.dir} --release --yes')

    def test_backup_invokes_installer_with_backup_only(self):
        sb = Sandbox(self)
        self.make_wrapper(sb)
        r = self.run_wrapper(sb, 'backup')
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertEqual(r.stdout.strip(), f'--dir {sb.dir} --backup-only')

    def test_rollback_restores_previous_ref_and_tags(self):
        sb = Sandbox(self)
        self.make_wrapper(sb)
        r = self.run_wrapper(sb, 'rollback')
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertIn('Rolling back to main (commit aaa111)', r.stdout)
        self.assertEqual(r.stdout.strip().splitlines()[-1],
                         f'--dj --promoter --dir {sb.dir} --yes '
                         '--ref aaa111 --dj-tag sha-old123 --promoter-tag sha-old456')


if __name__ == '__main__':
    unittest.main()
