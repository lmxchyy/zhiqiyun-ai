import test from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import { execFileSync, spawnSync } from 'node:child_process';
const root = new URL('../', import.meta.url);
const bash = process.platform === 'win32' ? 'C:/Program Files/Git/bin/bash.exe' : 'bash';

test('R4 status query failure cannot reach migration (original production shell block)', async () => {
  const text = await readFile(new URL('deploy.sh', root), 'utf8');
  const block = text.slice(text.indexOf('  # Record old container IDs before stopping'), text.indexOf('  # Execute database migration'));
  assert.ok(block.length > 100);
  for (const failed of ['ps', 'State.Running', 'State.ExitCode', 'stop']) {
    const script = `set -Eeuo pipefail
COMPOSE_FILE=test
ENV_FILE=test
cur_api_cid=api
cur_worker_cid=worker
fail() { echo "$*" >&2; exit 1; }
log() { :; }
docker() {
 case " $* " in
 *"${failed}"*) return 42 ;;
 *" ps --status "*) return 0 ;;
 *" ps "*smartvideo-worker*) echo worker ;;
 *" ps "*) echo api ;;
 *"State.Running"*) echo false ;;
 *"State.ExitCode"*) echo 0 ;;
 *"State.Status"*) echo exited ;;
 *) return 0 ;;
 esac
}
${block}
echo REACHED_MIGRATION
`;
    const r = spawnSync(bash, ['-s'], { input: script, encoding: 'utf8' });
    assert.notEqual(r.status, 0, `${failed} was treated as safe: ${r.stdout}`);
    assert.doesNotMatch(r.stderr, /identity changed/);
    assert.doesNotMatch(r.stdout, /REACHED_MIGRATION/);
  }
});

test('R4 proof hash binds full effective Compose incl secret env, endpoints and mount sources', async () => {
  const script = await readFile(new URL('ops/verify-prestage-proof.sh', root), 'utf8');
  // Execute the actual hash policy, not a copy of the algorithm.
  const start = script.indexOf('services_data = actual_compose_data.get(');
  const end = script.indexOf('expected_bound_hash =', start);
  assert.ok(start >= 0 && end > start);
  const policy = script.slice(start, end);
  const py = `import json,hashlib,copy\npolicy=${JSON.stringify(policy)}\ndef digest(d):\n n={'json':json,'hashlib':hashlib,'actual_compose_data':d,'fail':lambda msg: (_ for _ in ()).throw(Exception(msg))}\n exec(policy,n)\n return n['actual_bound_hash']\nbase={'services':{s:{'image':'image@sha256:abc','environment':{},'volumes':[{'source':'/trusted','target':'/data'}]} for s in ['xianzhi-ai','smartvideo-worker','migrate']}}\nfor key in ['DATABASE_URL','POSTGRES_DB','RABBITMQ_URL','GENERATION_ASYNC_CANARY_ENABLED','GENERATION_ASYNC_CANARY_USERS','GENERATION_ASYNC_CANARY_MODEL_ALLOWLIST','S3_BUCKET','S3_REGION','OBS_PREFIX','VIDEO_STORAGE_PERSISTENCE_ENABLED']:\n changed=copy.deepcopy(base);changed['services']['xianzhi-ai']['environment'][key]='changed'\n assert digest(base)!=digest(changed),key+' unbound'\nchanged=copy.deepcopy(base);changed['services']['migrate']['volumes'][0]['source']='/different'\nassert digest(base)!=digest(changed),'mount source unbound'\nprint('ALL_CONFIG_DRIFT_REJECTED')\n`;
  const r = spawnSync('python3', ['-c', py], { encoding: 'utf8' });
  assert.equal(r.status, 0, r.stderr);
});

test('R4 explicit runtime gate is connected before drain and before success', async () => {
  const text = await readFile(new URL('deploy.sh', root), 'utf8');
  assert.match(text, /python3 ops\/verify-release-runtime\.py pre/);
  assert.match(text, /python3 ops\/verify-release-runtime\.py post/);
  const preIdx = text.indexOf('python3 ops/verify-release-runtime.py pre');
  const postIdx = text.indexOf('python3 ops/verify-release-runtime.py post');
  const drainCallIdx = text.indexOf('check_safe_drain', preIdx);
  const ledgerIdx = text.indexOf('Recording successful deployment', postIdx);
  assert.ok(preIdx >= 0 && drainCallIdx > preIdx, 'pre gate must run before check_safe_drain');
  assert.ok(postIdx >= 0 && ledgerIdx > postIdx, 'post gate must run before recording ledger success');
});

test('R4 actual runtime probe rejects query failure, unhealthy scheduler, missing/zero consumers', () => {
  const modulePath = new URL('ops/verify-release-runtime.py', root).pathname.replace(/^\/([A-Za-z]:)/, '$1');
  const py = `import importlib.util,json,sys\ns=importlib.util.spec_from_file_location('gate',sys.argv[1]);g=importlib.util.module_from_spec(s);s.loader.exec_module(g)\nmetrics='generation_scheduler_db_scrape_success 1\\ngeneration_scheduler_errors_total 0\\ngeneration_scheduler_dispatched_total 0\\ngeneration_scheduler_recovered_total 0\\n'\nqueues=[dict(name='x.ai.generation.'+n,consumers=1,messages_ready=0,messages_unacknowledged=0) for n in ['image.normal','image.canary','video.canary','ppt.canary']]\ndef runner(mode):\n def run(args):\n  text=' '.join(args)\n  if '/metrics' in text:\n   if mode=='scheduler-query': raise RuntimeError('injected query failure')\n   return metrics.replace('scrape_success 1','scrape_success 0') if mode=='scheduler-unhealthy' else metrics\n  if '/ready' in text: return json.dumps(dict(ready='true',asyncMessaging='READY'))\n  if 'python3' in args:\n   if 'urllib' in args[-1]:\n    if mode=='consumer-query': raise RuntimeError('injected query failure')\n    q=json.loads(json.dumps(queues))\n    if mode=='missing-consumer': q=q[1:]\n    if mode=='zero-consumer': q[0]['consumers']=0\n    if mode=='active-queue': q[0]['messages_unacknowledged']=1\n    return json.dumps(q)\n   return json.dumps(dict(GENERATION_FAIR_SCHEDULER_ENABLED='true',ASYNC_MESSAGING_ENABLED='true'))\n  if 'ps' in args: return 'cid'\n  if 'Health.Status' in text: return 'healthy'\n  if 'State.Running' in text: return 'true'\n  raise AssertionError('unhandled probe')\n return run\ng.verify('post','compose','env',runner('ok'),lambda _:None)\nfor mode in ['scheduler-query','scheduler-unhealthy','consumer-query','missing-consumer','zero-consumer','active-queue']:\n try: g.verify('pre' if mode=='active-queue' else 'post','compose','env',runner(mode),lambda _:None)\n except Exception: print(mode+': rejected')\n else: raise AssertionError(mode+' passed')\n`;
  const result = spawnSync('python3', ['-c', py, decodeURIComponent(modulePath)], { encoding: 'utf8' });
  assert.equal(result.status, 0, result.stderr);
});
