export declare function encodeVarint(n: number): Uint8Array;
/** Decodes a varint at offset. Returns the value and the number of bytes read. */
export declare function decodeVarint(buf: Uint8Array, offset?: number): [value: number, length: number];
