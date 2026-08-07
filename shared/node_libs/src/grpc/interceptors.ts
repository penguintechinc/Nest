/**
 * gRPC security interceptors for authentication, rate limiting, and audit logging.
 *
 * These implement the @grpc/grpc-js *server*-side `ServerInterceptor` contract
 * (`(methodDescriptor, call) => ServerInterceptingCall`), which is distinct from
 * the client-side `Interceptor` contract (`(options, nextCall) => InterceptingCall`).
 * Incoming request data (headers, auth tokens) is observed/mutated via the
 * `start` responder hook's `onReceiveMetadata` listener; outgoing response
 * status is observed via the `sendStatus` responder hook. See
 * `ServerInterceptingCall` in `@grpc/grpc-js` for the underlying mechanism.
 */

import * as grpc from '@grpc/grpc-js';
import * as jwt from 'jsonwebtoken';
import { v4 as uuidv4 } from 'uuid';

/**
 * JWT authentication interceptor for gRPC servers.
 *
 * Validates JWT tokens in metadata and sets user context.
 *
 * @example
 * ```typescript
 * const server = createServer([
 *   authInterceptor('your-secret-key', ['/health.Check']),
 * ]);
 * ```
 */
export function authInterceptor(
  secretKey: string,
  publicMethods: string[] = []
): grpc.ServerInterceptor {
  const publicMethodSet = new Set(publicMethods);

  return (methodDescriptor, call) => {
    const method = methodDescriptor.path;

    return new grpc.ServerInterceptingCall(call, {
      start(next) {
        next({
          onReceiveMetadata(metadata, next) {
            // Skip auth for public methods
            if (publicMethodSet.has(method)) {
              next(metadata);
              return;
            }

            // Extract token from metadata
            const authHeader = metadata.get('authorization')[0] as string | undefined;

            if (!authHeader || !authHeader.startsWith('Bearer ')) {
              call.sendStatus({
                code: grpc.status.UNAUTHENTICATED,
                details: 'Missing or invalid authorization header',
              });
              return;
            }

            const token = authHeader.substring(7); // Remove 'Bearer ' prefix

            try {
              // Validate JWT token
              const payload = jwt.verify(token, secretKey) as jwt.JwtPayload;

              // Add user info to metadata
              if (payload.sub) {
                metadata.set('user-id', payload.sub);
                console.log(`Authenticated request to ${method} from user ${payload.sub}`);
              }

              next(metadata);
            } catch (error) {
              console.warn(`Invalid token for ${method}:`, error);
              call.sendStatus({
                code: grpc.status.UNAUTHENTICATED,
                details: 'Invalid token',
              });
            }
          },
        });
      },
    });
  };
}

interface RateLimitEntry {
  count: number;
  windowStart: number;
}

/**
 * Rate limiting interceptor with per-client limits.
 *
 * Implements sliding window rate limiting.
 *
 * @example
 * ```typescript
 * const server = createServer([
 *   rateLimitInterceptor(100, true),
 * ]);
 * ```
 */
export function rateLimitInterceptor(
  requestsPerMinute: number = 100,
  perUser: boolean = true
): grpc.ServerInterceptor {
  const limits = new Map<string, RateLimitEntry>();

  return (_methodDescriptor, call) => {
    return new grpc.ServerInterceptingCall(call, {
      start(next) {
        next({
          onReceiveMetadata(metadata, next) {
            // Determine client identifier
            let clientId = 'anonymous';

            if (perUser) {
              // Extract user from token
              const authHeader = metadata.get('authorization')[0] as string | undefined;
              if (authHeader && authHeader.startsWith('Bearer ')) {
                try {
                  const token = authHeader.substring(7);
                  const payload = jwt.decode(token) as jwt.JwtPayload | null;
                  if (payload?.sub) {
                    clientId = payload.sub;
                  }
                } catch {
                  // Ignore decode errors
                }
              }
            } else {
              // Use peer address (IP)
              const forwarded = metadata.get('x-forwarded-for')[0] as string | undefined;
              if (forwarded) {
                clientId = forwarded;
              }
            }

            // Check rate limit
            const currentTime = Date.now();
            let entry = limits.get(clientId);

            if (!entry) {
              entry = { count: 0, windowStart: currentTime };
              limits.set(clientId, entry);
            }

            // Reset window if expired
            if (currentTime - entry.windowStart >= 60000) {
              entry.count = 0;
              entry.windowStart = currentTime;
            }

            // Check limit
            if (entry.count >= requestsPerMinute) {
              console.warn(`Rate limit exceeded for ${clientId}`, {
                requests: entry.count,
              });

              call.sendStatus({
                code: grpc.status.RESOURCE_EXHAUSTED,
                details: 'Rate limit exceeded',
              });
              return;
            }

            // Increment counter
            entry.count++;

            next(metadata);
          },
        });
      },
    });
  };
}

/**
 * Audit logging interceptor for request/response tracking.
 *
 * Logs method calls, duration, and status codes.
 *
 * @example
 * ```typescript
 * const server = createServer([
 *   auditInterceptor(),
 * ]);
 * ```
 */
export function auditInterceptor(): grpc.ServerInterceptor {
  return (methodDescriptor, call) => {
    const method = methodDescriptor.path;
    const startTime = Date.now();
    let correlationId = 'unknown';

    return new grpc.ServerInterceptingCall(call, {
      start(next) {
        next({
          onReceiveMetadata(metadata, next) {
            correlationId = (metadata.get('x-correlation-id')[0] as string) || 'unknown';

            console.log(`gRPC request started: ${method}`, {
              method,
              correlationId,
            });

            next(metadata);
          },
        });
      },
      sendStatus(status, next) {
        const durationMs = Date.now() - startTime;

        if (status.code === grpc.status.OK) {
          console.log(`gRPC request completed: ${method}`, {
            method,
            durationMs,
            correlationId,
            status: 'OK',
          });
        } else {
          console.error(`gRPC request failed: ${method}`, {
            method,
            durationMs,
            correlationId,
            code: status.code,
            details: status.details,
          });
        }

        next(status);
      },
    });
  };
}

/**
 * Correlation ID interceptor for request tracing.
 *
 * Adds or propagates correlation IDs across service calls.
 *
 * @example
 * ```typescript
 * const server = createServer([
 *   correlationInterceptor(),
 * ]);
 * ```
 */
export function correlationInterceptor(): grpc.ServerInterceptor {
  return (_methodDescriptor, call) => {
    return new grpc.ServerInterceptingCall(call, {
      start(next) {
        next({
          onReceiveMetadata(metadata, next) {
            // Get or create correlation ID
            let correlationId = metadata.get('x-correlation-id')[0] as string | undefined;
            if (!correlationId) {
              correlationId = uuidv4();
              metadata.set('x-correlation-id', correlationId);
              console.debug(`Generated new correlation ID: ${correlationId}`);
            }

            next(metadata);
          },
        });
      },
    });
  };
}
