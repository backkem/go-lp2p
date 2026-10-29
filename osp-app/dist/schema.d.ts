import { type CBORValue } from "./codec/cbor.ts";
/** A message or field doesn't match its CDDL. */
export declare class SchemaError extends TypeError {
    name: string;
}
export interface Codec<T> {
    /** CDDL-ish description, for error messages. */
    readonly desc: string;
    encode(v: T, path: string): CBORValue;
    decode(v: CBORValue, path: string): T;
}
export type Infer<C> = C extends Codec<infer T> ? T : never;
export declare const uint: Codec<number>;
export declare const int: Codec<number>;
/** float64: always sent as a CBOR float64; integers are accepted on receipt. */
export declare const float64: Codec<number>;
export declare const text: Codec<string>;
export declare const bytes: Codec<Uint8Array>;
export declare const bool: Codec<boolean>;
/** "bytes / text": a string is a text message, a Uint8Array a binary one. */
export declare const bytesOrText: Codec<string | Uint8Array>;
/** Any CBOR value, passed through untouched (used for out-of-scope groups). */
export declare const any: Codec<CBORValue>;
/** &( name: value ... ): a closed set of uint values. */
export declare function choice<const V extends number>(values: Record<string, V>): Codec<V>;
export declare function nullable<T>(c: Codec<T>): Codec<T | null>;
/** [min* T] */
export declare function array<T>(c: Codec<T>, min?: number): Codec<T[]>;
/** A fixed-length CDDL array, such as http-header = [key: text, value: text]. */
export declare function tuple<const T extends unknown[]>(...cs: {
    [K in keyof T]: Codec<T[K]>;
}): Codec<T>;
export interface Field<T, Optional extends boolean> {
    readonly key: number;
    readonly codec: Codec<T>;
    readonly optional: Optional;
}
/** `key: T` */
export declare function req<T>(key: number, codec: Codec<T>): Field<T, false>;
/** `? key: T` */
export declare function opt<T>(key: number, codec: Codec<T>): Field<T, true>;
type Fields = Record<string, Field<any, boolean>>;
type Simplify<T> = {
    [K in keyof T]: T[K];
} & {};
export type StructOf<F extends Fields> = Simplify<{
    -readonly [K in keyof F as F[K]["optional"] extends true ? never : K]: Infer<F[K]["codec"]>;
} & {
    -readonly [K in keyof F as F[K]["optional"] extends true ? K : never]?: Infer<F[K]["codec"]>;
}>;
/**
 * A CDDL map with integer keys, as a typed object. Unknown keys are ignored on
 * receipt (application.bs allows extension fields); an optional property that
 * is undefined is left out.
 */
export declare function struct<const F extends Fields>(fields: F): Codec<StructOf<F>> & {
    readonly fields: F;
};
export {};
