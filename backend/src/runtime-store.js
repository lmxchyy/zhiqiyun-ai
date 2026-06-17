import pg from "pg";
import { createStore, Store } from "./store.js";

const { Pool } = pg;
const STATE_ID = "main";

export class PostgresStateStore extends Store {
  constructor(pool, state, version = 0) {
    super({ persist: false });
    this.pool = pool;
    this.state = { ...this.state, ...state };
    this.version = Number(version || 0);
    this.backend = "postgres";
    this.dirty = false;
    this.flushing = null;
  }

  save() {
    this.dirty = true;
  }

  async refresh() {
    await this.flush();
    const result = await this.pool.query("select state, version from platform_state where id = $1", [STATE_ID]);
    if (result.rows[0]) {
      this.state = { ...createStore({ persist: false }).state, ...result.rows[0].state };
      this.version = Number(result.rows[0].version || 0);
    }
    return this.state;
  }

  async flush() {
    if (this.flushing) return this.flushing;
    if (!this.dirty) return;
    const snapshot = structuredClone(this.state);
    this.dirty = false;
    this.flushing = this.pool.query(
      `insert into platform_state (id, state, version, updated_at)
       values ($1, $2::jsonb, 1, now())
       on conflict (id) do update
       set state = excluded.state, version = platform_state.version + 1, updated_at = now()
       returning version`,
      [STATE_ID, JSON.stringify(snapshot)]
    ).then((result) => {
      this.version = Number(result.rows[0].version);
    }).catch((error) => {
      this.dirty = true;
      throw error;
    }).finally(() => {
      this.flushing = null;
    });
    return this.flushing;
  }
}

export async function createRuntimeStore() {
  if (!process.env.DATABASE_URL) {
    const store = createStore();
    store.backend = "file";
    store.flush = async () => {};
    return store;
  }

  const pool = new Pool({ connectionString: process.env.DATABASE_URL, max: 5 });
  await pool.query("select 1");
  const result = await pool.query("select state, version from platform_state where id = $1", [STATE_ID]);
  if (result.rows[0]) return new PostgresStateStore(pool, result.rows[0].state, result.rows[0].version);

  const legacy = createStore();
  const store = new PostgresStateStore(pool, legacy.state, 0);
  store.dirty = true;
  await store.flush();
  return store;
}
