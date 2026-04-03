/**
 * WHATWG Server-Sent Events parser.
 *
 * Vendored from eventsource-parser v3.0.6 (MIT License)
 * Copyright (c) 2025 Espen Hovlandsdal <espen@hovlandsdal.com>
 * https://github.com/rexxars/eventsource-parser
 *
 * Consolidated from parse.ts, types.ts, and errors.ts into a single file.
 * The SSE spec is frozen (2015) — no maintenance risk from vendoring.
 *
 * @see https://html.spec.whatwg.org/multipage/server-sent-events.html
 */
/** The type of parse error that occurred. */
export type ErrorType = 'invalid-retry' | 'unknown-field';
/** Error thrown when encountering an issue during SSE parsing. */
export declare class ParseError extends Error {
    type: ErrorType;
    field?: string | undefined;
    value?: string | undefined;
    line?: string | undefined;
    constructor(message: string, options: {
        type: ErrorType;
        field?: string;
        value?: string;
        line?: string;
    });
}
/** A parsed EventSource message event. */
export interface EventSourceMessage {
    /** Event type from the "event:" field. Undefined if not set (defaults to "message" in browsers). */
    event?: string | undefined;
    /** Event ID from the "id:" field. Used for reconnection via Last-Event-ID header. */
    id?: string | undefined;
    /** The event data from "data:" field(s). Multi-line data is joined with "\n". */
    data: string;
}
/** EventSource parser instance returned by createParser. */
export interface EventSourceParser {
    /** Feed the parser another chunk of SSE data. Triggers callbacks as events are parsed. */
    feed(chunk: string): void;
    /** Reset parser state. Required between reconnections. */
    reset(options?: {
        consume?: boolean;
    }): void;
}
/** Callbacks invoked by the parser during stream processing. */
export interface ParserCallbacks {
    /** Called when a complete event is parsed. */
    onEvent?: ((event: EventSourceMessage) => void) | undefined;
    /** Called when the server sends a reconnection interval via "retry:" field. */
    onRetry?: ((retry: number) => void) | undefined;
    /** Called when a comment line is encountered (lines starting with ":"). */
    onComment?: ((comment: string) => void) | undefined;
    /** Called when a parse error occurs (unknown fields, invalid retry values). */
    onError?: ((error: ParseError) => void) | undefined;
}
/**
 * Creates a new WHATWG-compliant EventSource parser.
 *
 * @param callbacks - Callbacks to invoke on parsing events
 * @returns Parser with `feed()` and `reset()` methods
 */
export declare function createParser(callbacks: ParserCallbacks): EventSourceParser;
//# sourceMappingURL=sse-parser.d.ts.map