export declare function encodeVarint(n: number): Uint8Array;
/**
 * Decodes a varint at offset and returns [value, length]. Throws
 * IncompleteError when buf ends inside the varint.
 */
export declare function decodeVarint(buf: Uint8Array, offset?: number): [value: number, length: number];
