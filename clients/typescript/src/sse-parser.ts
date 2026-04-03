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

// ============================================================================
// Types
// ============================================================================

/** The type of parse error that occurred. */
export type ErrorType = 'invalid-retry' | 'unknown-field';

/** Error thrown when encountering an issue during SSE parsing. */
export class ParseError extends Error {
  type: ErrorType;
  field?: string | undefined;
  value?: string | undefined;
  line?: string | undefined;

  constructor(
    message: string,
    options: { type: ErrorType; field?: string; value?: string; line?: string },
  ) {
    super(message);
    this.name = 'ParseError';
    this.type = options.type;
    this.field = options.field;
    this.value = options.value;
    this.line = options.line;
  }
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
  reset(options?: { consume?: boolean }): void;
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

// ============================================================================
// Parser
// ============================================================================

function noop(_arg: unknown) {
  // intentional noop
}

/**
 * Creates a new WHATWG-compliant EventSource parser.
 *
 * @param callbacks - Callbacks to invoke on parsing events
 * @returns Parser with `feed()` and `reset()` methods
 */
export function createParser(callbacks: ParserCallbacks): EventSourceParser {
  if (typeof callbacks === 'function') {
    throw new TypeError(
      '`callbacks` must be an object, got a function instead. Did you mean `{onEvent: fn}`?',
    );
  }

  const { onEvent = noop, onError = noop, onRetry = noop, onComment } = callbacks;

  let incompleteLine = '';
  let isFirstChunk = true;
  let id: string | undefined;
  let data = '';
  let eventType = '';

  function feed(newChunk: string) {
    const chunk = isFirstChunk ? newChunk.replace(/^\xEF\xBB\xBF/, '') : newChunk;
    const [complete, incomplete] = splitLines(`${incompleteLine}${chunk}`);

    for (const line of complete) {
      parseLine(line);
    }

    incompleteLine = incomplete;
    isFirstChunk = false;
  }

  function parseLine(line: string) {
    if (line === '') {
      dispatchEvent();
      return;
    }

    if (line.startsWith(':')) {
      if (onComment) {
        onComment(line.slice(line.startsWith(': ') ? 2 : 1));
      }
      return;
    }

    const fieldSeparatorIndex = line.indexOf(':');
    if (fieldSeparatorIndex !== -1) {
      const field = line.slice(0, fieldSeparatorIndex);
      const offset = line[fieldSeparatorIndex + 1] === ' ' ? 2 : 1;
      const value = line.slice(fieldSeparatorIndex + offset);
      processField(field, value, line);
      return;
    }

    processField(line, '', line);
  }

  function processField(field: string, value: string, line: string) {
    switch (field) {
      case 'event':
        eventType = value;
        break;
      case 'data':
        data = `${data}${value}\n`;
        break;
      case 'id':
        id = value.includes('\0') ? undefined : value;
        break;
      case 'retry':
        if (/^\d+$/.test(value)) {
          onRetry(parseInt(value, 10));
        } else {
          onError(
            new ParseError(`Invalid \`retry\` value: "${value}"`, {
              type: 'invalid-retry',
              value,
              line,
            }),
          );
        }
        break;
      default:
        onError(
          new ParseError(
            `Unknown field "${field.length > 20 ? `${field.slice(0, 20)}…` : field}"`,
            { type: 'unknown-field', field, value, line },
          ),
        );
        break;
    }
  }

  function dispatchEvent() {
    const shouldDispatch = data.length > 0;
    if (shouldDispatch) {
      onEvent({
        id,
        event: eventType || undefined,
        data: data.endsWith('\n') ? data.slice(0, -1) : data,
      });
    }

    id = undefined;
    data = '';
    eventType = '';
  }

  function reset(options: { consume?: boolean } = {}) {
    if (incompleteLine && options.consume) {
      parseLine(incompleteLine);
    }

    isFirstChunk = true;
    id = undefined;
    data = '';
    eventType = '';
    incompleteLine = '';
  }

  return { feed, reset };
}

// ============================================================================
// Line splitting (handles CR, LF, CRLF correctly per WHATWG spec)
// ============================================================================

function splitLines(chunk: string): [complete: Array<string>, incomplete: string] {
  const lines: Array<string> = [];
  let incompleteLine = '';
  let searchIndex = 0;

  while (searchIndex < chunk.length) {
    const crIndex = chunk.indexOf('\r', searchIndex);
    const lfIndex = chunk.indexOf('\n', searchIndex);

    let lineEnd = -1;
    if (crIndex !== -1 && lfIndex !== -1) {
      lineEnd = Math.min(crIndex, lfIndex);
    } else if (crIndex !== -1) {
      if (crIndex === chunk.length - 1) {
        lineEnd = -1;
      } else {
        lineEnd = crIndex;
      }
    } else if (lfIndex !== -1) {
      lineEnd = lfIndex;
    }

    if (lineEnd === -1) {
      incompleteLine = chunk.slice(searchIndex);
      break;
    } else {
      const line = chunk.slice(searchIndex, lineEnd);
      lines.push(line);

      searchIndex = lineEnd + 1;
      if (chunk[searchIndex - 1] === '\r' && chunk[searchIndex] === '\n') {
        searchIndex++;
      }
    }
  }

  return [lines, incompleteLine];
}
