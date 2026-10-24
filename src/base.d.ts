type callback<T> = (e: T) => void;
// Nullable, as in HTML: typedef EventHandlerNonNull? EventHandler.
type EventHandler<T> = callback<T> | null;

// https://developer.mozilla.org/en-US/docs/Web/JavaScript/Reference/Global_Objects/Symbol/asyncIterator
// https://developer.mozilla.org/en-US/docs/Web/API/ReadableStream#browser_compatibility
interface ReadableStream<R = any> {
    [Symbol.asyncIterator](): AsyncIterableIterator<R>;
}