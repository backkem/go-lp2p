// A tiny schema layer between CDDL and TypeScript. Each CDDL type is a Codec
// that converts between a typed JS value and the CBOR value model, checking
// shape and ranges both ways. struct() maps the CDDL's integer map keys to
// named, typed properties, and Infer<> derives the TypeScript type, so every
// message is declared once (in messages.ts) and type-checked everywhere.
import { Float64 } from "./codec/cbor.js";
/** A message or field doesn't match its CDDL. */
export class SchemaError extends TypeError {
    name = "SchemaError";
}
function fail(path, want, got) {
    const shown = got instanceof Map ? "map" : got instanceof Uint8Array ? "bytes" : Array.isArray(got) ? "array" : got instanceof Float64 ? "float" : JSON.stringify(got);
    throw new SchemaError(`${path}: expected ${want}, got ${shown}`);
}
function isUint(v) {
    return typeof v === "number" && Number.isSafeInteger(v) && v >= 0;
}
export const uint = {
    desc: "uint",
    encode(v, path) {
        if (!isUint(v))
            fail(path, "uint", v);
        return v;
    },
    decode(v, path) {
        if (!isUint(v))
            fail(path, "uint", v);
        return v;
    },
};
export const int = {
    desc: "int",
    encode(v, path) {
        if (typeof v !== "number" || !Number.isSafeInteger(v))
            fail(path, "int", v);
        return v;
    },
    decode(v, path) {
        if (typeof v !== "number" || !Number.isSafeInteger(v))
            fail(path, "int", v);
        return v;
    },
};
/** float64: always sent as a CBOR float64; integers are accepted on receipt. */
export const float64 = {
    desc: "float64",
    encode(v, path) {
        if (typeof v !== "number")
            fail(path, "float64", v);
        return new Float64(v);
    },
    decode(v, path) {
        if (v instanceof Float64)
            return v.value;
        if (typeof v === "number")
            return v;
        fail(path, "float64", v);
    },
};
export const text = {
    desc: "text",
    encode(v, path) {
        if (typeof v !== "string")
            fail(path, "text", v);
        return v;
    },
    decode(v, path) {
        if (typeof v !== "string")
            fail(path, "text", v);
        return v;
    },
};
export const bytes = {
    desc: "bytes",
    encode(v, path) {
        if (!(v instanceof Uint8Array))
            fail(path, "bytes", v);
        return v;
    },
    decode(v, path) {
        if (!(v instanceof Uint8Array))
            fail(path, "bytes", v);
        return v;
    },
};
export const bool = {
    desc: "bool",
    encode(v, path) {
        if (typeof v !== "boolean")
            fail(path, "bool", v);
        return v;
    },
    decode(v, path) {
        if (typeof v !== "boolean")
            fail(path, "bool", v);
        return v;
    },
};
/** "bytes / text": a string is a text message, a Uint8Array a binary one. */
export const bytesOrText = {
    desc: "bytes / text",
    encode(v, path) {
        if (typeof v !== "string" && !(v instanceof Uint8Array))
            fail(path, "bytes / text", v);
        return v;
    },
    decode(v, path) {
        if (typeof v !== "string" && !(v instanceof Uint8Array))
            fail(path, "bytes / text", v);
        return v;
    },
};
/** Any CBOR value, passed through untouched (used for out-of-scope groups). */
export const any = {
    desc: "any",
    encode: (v) => v,
    decode: (v) => v,
};
/** &( name: value ... ): a closed set of uint values. */
export function choice(values) {
    const allowed = new Set(Object.values(values));
    const desc = `&(${Object.entries(values)
        .map(([k, v]) => `${k}: ${v}`)
        .join(", ")})`;
    const check = (v, path) => {
        if (!isUint(v) || !allowed.has(v))
            fail(path, desc, v);
        return v;
    };
    return { desc, encode: check, decode: check };
}
export function nullable(c) {
    return {
        desc: `${c.desc} / null`,
        encode: (v, path) => (v === null ? null : c.encode(v, path)),
        decode: (v, path) => (v === null ? null : c.decode(v, path)),
    };
}
/** [min* T] */
export function array(c, min = 0) {
    const desc = `[${min || "*"}${min ? "*" : ""} ${c.desc}]`;
    return {
        desc,
        encode(v, path) {
            if (!Array.isArray(v) || v.length < min)
                fail(path, desc, v);
            return v.map((item, i) => c.encode(item, `${path}[${i}]`));
        },
        decode(v, path) {
            if (!Array.isArray(v) || v.length < min)
                fail(path, desc, v);
            return v.map((item, i) => c.decode(item, `${path}[${i}]`));
        },
    };
}
/** A fixed-length CDDL array, such as http-header = [key: text, value: text]. */
export function tuple(...cs) {
    const desc = `[${cs.map((c) => c.desc).join(", ")}]`;
    return {
        desc,
        encode(v, path) {
            if (!Array.isArray(v) || v.length !== cs.length)
                fail(path, desc, v);
            return cs.map((c, i) => c.encode(v[i], `${path}[${i}]`));
        },
        decode(v, path) {
            if (!Array.isArray(v) || v.length !== cs.length)
                fail(path, desc, v);
            return cs.map((c, i) => c.decode(v[i], `${path}[${i}]`));
        },
    };
}
/** `key: T` */
export function req(key, codec) {
    return { key, codec, optional: false };
}
/** `? key: T` */
export function opt(key, codec) {
    return { key, codec, optional: true };
}
/**
 * A CDDL map with integer keys, as a typed object. Unknown keys are ignored on
 * receipt (application.bs allows extension fields); an optional property that
 * is undefined is left out.
 */
export function struct(fields) {
    const entries = Object.entries(fields);
    const desc = `{${entries.map(([name, f]) => `${f.optional ? "? " : ""}${f.key}: ${name}`).join(", ")}}`;
    return {
        desc,
        fields,
        encode(v, path) {
            if (typeof v !== "object" || v === null)
                fail(path, desc, v);
            const out = new Map();
            const obj = v;
            for (const [name, f] of entries) {
                const val = obj[name];
                if (val === undefined) {
                    if (!f.optional)
                        throw new SchemaError(`${path}.${name}: required field (key ${f.key}) is missing`);
                    continue;
                }
                out.set(f.key, f.codec.encode(val, `${path}.${name}`));
            }
            return out;
        },
        decode(v, path) {
            if (!(v instanceof Map))
                fail(path, desc, v);
            const out = {};
            for (const [name, f] of entries) {
                const val = v.get(f.key);
                if (val === undefined) {
                    if (!f.optional)
                        throw new SchemaError(`${path}.${name}: required field (key ${f.key}) is missing`);
                    continue;
                }
                out[name] = f.codec.decode(val, `${path}.${name}`);
            }
            return out;
        },
    };
}
