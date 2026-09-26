import { spawn } from 'node:child_process';
import { readFileSync } from 'node:fs';
import { pathToFileURL } from 'node:url';

// nuxt-nats takes the user JWT and NKey seed as strings; deployments mount an
// nsc .creds file instead. Parse it here so the seed never enters compose env.
export function natsEnvFromCreds(path) {
  if (!path) return {};
  const text = readFileSync(path, 'utf8');
  const jwt = /-----BEGIN NATS USER JWT-----\s+([^\s]+)\s+------END NATS USER JWT------/.exec(text)?.[1];
  const seed = /-----BEGIN USER NKEY SEED-----\s+([^\s]+)\s+------END USER NKEY SEED------/.exec(text)?.[1];
  if (!jwt || !seed) throw new Error(`invalid NATS creds file: ${path}`);
  return { NUXT_NATS_USER_JWT: jwt, NUXT_NATS_NKEY_SEED: seed };
}

// tini is PID 1 and reaps orphaned Chromium processes. Each service gets its
// own process group so shutdown also reaches browser descendants.
export function supervise({
  apiCommand = ['/promoter', 'serve'],
  frontendCommand = [process.execPath, '/opt/frontend/server/index.mjs'],
  shutdownTimeoutMs = (Number(process.env.SHUTDOWN_TIMEOUT_SEC) || 30) * 1000 +
    1000,
} = {}) {
  return new Promise((resolve) => {
    const children = new Set();
    let stopping = false;
    let exitCode = 0;
    let timer;
    const signalGroup = (child, signal) => {
      if (!child.pid) return;
      try {
        process.kill(-child.pid, signal);
      } catch (error) {
        if (error.code !== 'ESRCH') console.error(error);
      }
    };
    const finish = () => {
      if (!stopping || children.size) return;
      clearTimeout(timer);
      process.off('SIGTERM', onTerm);
      process.off('SIGINT', onInt);
      resolve(exitCode);
    };
    const stop = (signal, code) => {
      if (stopping) return;
      stopping = true;
      exitCode = code;
      for (const child of children) signalGroup(child, signal);
      timer = setTimeout(() => {
        console.error('[runtime] shutdown deadline exceeded; killing services');
        for (const child of children) signalGroup(child, 'SIGKILL');
      }, shutdownTimeoutMs);
      finish();
    };
    const onTerm = () => stop('SIGTERM', 0);
    const onInt = () => stop('SIGINT', 0);
    process.on('SIGTERM', onTerm);
    process.on('SIGINT', onInt);

    const start = (name, command, env) => {
      const child = spawn(command[0], command.slice(1), {
        env,
        stdio: 'inherit',
        detached: true,
      });
      children.add(child);
      child.on('error', (error) => {
        console.error(`[runtime] ${name} failed to start: ${error.message}`);
        stop('SIGTERM', 1);
      });
      child.on('exit', (code, signal) => {
        if (!stopping) {
          console.error(
            `[runtime] ${name} exited unexpectedly (${signal || code})`,
          );
          stop('SIGTERM', 1);
        }
      });
      child.on('close', () => {
        // Ensure descendants cannot survive a service exiting first.
        signalGroup(child, 'SIGKILL');
        children.delete(child);
        finish();
      });
    };
    start('api', apiCommand, {
      ...process.env,
      PROMOTER_BIND_ADDRESS: '0.0.0.0',
      PROMOTER_PORT: '8080',
      PROMOTER_SERVE_FRONTEND: 'true',
      PROMOTER_NUXT_INTERNAL_URL: 'http://127.0.0.1:3000',
    });
    start('frontend', frontendCommand, {
      ...process.env,
      ...natsEnvFromCreds(process.env.NUXT_NATS_CREDS_FILE),
      HOST: '127.0.0.1',
      PORT: '3000',
      NITRO_HOST: '127.0.0.1',
      NITRO_PORT: '3000',
    });
  });
}

if (
  process.argv[1] &&
  import.meta.url === pathToFileURL(process.argv[1]).href
) {
  process.exitCode = await supervise();
}
