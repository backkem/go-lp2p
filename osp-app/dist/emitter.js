/** A minimal typed event emitter. Listener errors are reported, not thrown into the emitter. */
export class Emitter {
    #listeners = new Map();
    /** Adds a listener and returns a function that removes it. */
    on(type, fn) {
        let set = this.#listeners.get(type);
        if (!set)
            this.#listeners.set(type, (set = new Set()));
        set.add(fn);
        return () => set.delete(fn);
    }
    /** Resolves with the next event of this type. */
    once(type) {
        return new Promise((resolve) => {
            const off = this.on(type, (v) => {
                off();
                resolve(v);
            });
        });
    }
    emit(type, v) {
        const set = this.#listeners.get(type);
        if (!set || set.size === 0)
            return false;
        for (const fn of [...set]) {
            try {
                fn(v);
            }
            catch (err) {
                queueMicrotask(() => {
                    throw err;
                });
            }
        }
        return true;
    }
}
