import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { once } from 'node:events';
import { test } from 'node:test';

const runtimeURL = new URL('./runtime.mjs', import.meta.url).href;
const worker = `console.log(JSON.stringify({ready:true,role:process.argv[1],host:process.env.HOST,port:process.env.PORT,bind:process.env.BIND_ADDRESS,nitro:process.env.NITRO_PORT})); process.on('SIGTERM',()=>{console.log(process.argv[1]+':SIGTERM');process.exit(0)}); process.on('SIGINT',()=>{console.log(process.argv[1]+':SIGINT');process.exit(0)}); setInterval(()=>{},1000);`;

function launch(overrides = {}) {
  const options = {
    apiCommand: [process.execPath, '-e', worker, 'api'],
    frontendCommand: [process.execPath, '-e', worker, 'frontend'],
    shutdownTimeoutMs: 300,
    ...overrides,
  };
  const source = `import { supervise } from ${JSON.stringify(runtimeURL)}; process.exitCode = await supervise(${JSON.stringify(options)});`;
  const child = spawn(process.execPath, ['--input-type=module', '-e', source], {
    env: { ...process.env, PORT: '9999', HOST: '0.0.0.0', NITRO_PORT: '9998' },
  });
  let output = '';
  child.stdout.on('data', (data) => {
    output += data;
  });
  child.stderr.on('data', (data) => {
    output += data;
  });
  const done = once(child, 'exit');
  return { child, done, output: () => output };
}

async function ready(proc) {
  const deadline = Date.now() + 5000;
  while ((proc.output().match(/"ready":true/g) || []).length < 2) {
    if (proc.child.exitCode !== null || Date.now() > deadline)
      throw new Error(proc.output());
    await new Promise((resolve) => setTimeout(resolve, 10));
  }
}

for (const signal of ['SIGTERM', 'SIGINT']) {
  test(
    `supervisor fixes child addresses and forwards ${signal}`,
    { timeout: 10000 },
    async (t) => {
      const proc = launch();
      t.after(() => {
        if (proc.child.exitCode === null) proc.child.kill('SIGKILL');
      });
      await ready(proc);
      const lines = proc
        .output()
        .trim()
        .split('\n')
        .filter((line) => line.startsWith('{'))
        .map(JSON.parse);
      assert.equal(lines.find((x) => x.role === 'api').bind, '0.0.0.0');
      assert.equal(lines.find((x) => x.role === 'api').port, '8080');
      assert.equal(lines.find((x) => x.role === 'frontend').host, '127.0.0.1');
      assert.equal(lines.find((x) => x.role === 'frontend').port, '3000');
      assert.equal(lines.find((x) => x.role === 'frontend').nitro, '3000');
      proc.child.kill(signal);
      assert.equal((await proc.done)[0], 0, proc.output());
      assert.match(proc.output(), new RegExp(`api:${signal}`));
      assert.match(proc.output(), new RegExp(`frontend:${signal}`));
    },
  );
}

for (const role of ['api', 'frontend']) {
  test(
    `unexpected ${role} clean exit still fails runtime`,
    { timeout: 10000 },
    async () => {
      const proc = launch({
        [`${role}Command`]: [
          process.execPath,
          '-e',
          'setTimeout(()=>process.exit(0),200)',
        ],
      });
      assert.equal((await proc.done)[0], 1, proc.output());
      assert.match(proc.output(), /SIGTERM/);
    },
  );
}

test(
  'spawn failure shuts down the other child',
  { timeout: 10000 },
  async () => {
    const proc = launch({ apiCommand: ['/nonexistent-klubhub-api'] });
    assert.equal((await proc.done)[0], 1, proc.output());
    assert.match(proc.output(), /api failed to start:.*ENOENT/);
  },
);

test(
  'unresponsive child is killed after shutdown deadline',
  { timeout: 10000 },
  async () => {
    const proc = launch({
      apiCommand: [
        process.execPath,
        '-e',
        "process.on('SIGTERM',()=>{});setInterval(()=>{},1000)",
      ],
      frontendCommand: [
        process.execPath,
        '-e',
        'setTimeout(()=>process.exit(2),200)',
      ],
    });
    assert.equal((await proc.done)[0], 1, proc.output());
    assert.match(proc.output(), /shutdown deadline exceeded/);
  },
);
