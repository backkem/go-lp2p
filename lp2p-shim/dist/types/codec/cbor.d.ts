export type CBORValue = number | string | boolean | null | undefined | Uint8Array | CBORValue[] | {
    [key: string]: CBORValue;
};
export declare function encodeCBOR(v: CBORValue): Uint8Array;
/** Decodes one CBOR item starting at offset. Returns the value and the offset after it. */
export declare function decodeCBORPrefix(buf: Uint8Array, offset?: number): [value: CBORValue, end: number];
/** Decodes exactly one CBOR item; trailing bytes are an error. */
export declare function decodeCBOR(buf: Uint8Array): CBORValue;
