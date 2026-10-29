/** A minimal typed event emitter. Listener errors are reported, not thrown into the emitter. */
export class Emitter<E extends { [K in keyof E]: unknown }> {
  readonly #listeners = new Map<keyof E, Set<(v: never) => void>>();

  /** Adds a listener and returns a function that removes it. */
  on<K extends keyof E>(type: K, fn: (v: E[K]) => void): () => void {
    let set = this.#listeners.get(type);
    if (!set) this.#listeners.set(type, (set = new Set()));
    set.add(fn as (v: never) => void);
    return () => set.delete(fn as (v: never) => void);
  }

  /** Resolves with the next event of this type. */
  once<K extends keyof E>(type: K): Promise<E[K]> {
    return new Promise((resolve) => {
      const off = this.on(type, (v) => {
        off();
        resolve(v);
      });
    });
  }

  protected emit<K extends keyof E>(type: K, v: E[K]): boolean {
    const set = this.#listeners.get(type);
    if (!set || set.size === 0) return false;
    for (const fn of [...set]) {
      try {
        (fn as (v: E[K]) => void)(v);
      } catch (err) {
        queueMicrotask(() => {
          throw err;
        });
      }
    }
    return true;
  }
}
