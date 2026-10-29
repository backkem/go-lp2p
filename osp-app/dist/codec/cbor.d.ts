/** A float that encodes as a CBOR float64 even when it is integral. */
export declare class Float64 {
    readonly value: number;
    constructor(value: number);
}
export type CBORKey = number | string;
export type CBORValue = number | Float64 | string | boolean | null | undefined | Uint8Array | CBORValue[] | Map<CBORKey, CBORValue>;
/** Thrown when the input ends inside an item: more bytes may complete it. */
export declare class IncompleteError extends RangeError {
    name: string;
}
export declare function encodeCBOR(v: CBORValue): Uint8Array;
/**
 * Decodes one item starting at offset and returns [value, end offset].
 * Throws IncompleteError if buf ends inside the item.
 */
export declare function decodeCBORPrefix(buf: Uint8Array, offset?: number): [value: CBORValue, end: number];
/** Decodes exactly one item; trailing bytes are an error. */
export declare function decodeCBOR(buf: Uint8Array): CBORValue;
