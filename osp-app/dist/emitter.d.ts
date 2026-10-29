/** A minimal typed event emitter. Listener errors are reported, not thrown into the emitter. */
export declare class Emitter<E extends {
    [K in keyof E]: unknown;
}> {
    #private;
    /** Adds a listener and returns a function that removes it. */
    on<K extends keyof E>(type: K, fn: (v: E[K]) => void): () => void;
    /** Resolves with the next event of this type. */
    once<K extends keyof E>(type: K): Promise<E[K]>;
    protected emit<K extends keyof E>(type: K, v: E[K]): boolean;
}
