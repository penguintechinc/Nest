import * as argon2 from 'argon2';

/**
 * Argon2 options accepted by this module, excluding `raw`.
 * This library only ever produces the encoded string form of a hash (never
 * a raw Buffer), so `raw` is intentionally excluded rather than pinned to
 * `false` — omitting the property entirely is what lets these options match
 * the string-returning `argon2.hash()` overload under
 * `exactOptionalPropertyTypes`.
 */
type HashOptions = Omit<argon2.Options, 'raw'>;

/**
 * Default options for Argon2 hashing.
 * These provide a good balance of security and performance for most applications.
 */
const DEFAULT_OPTIONS: HashOptions = {
  type: argon2.argon2id, // Hybrid mode (resistant to both GPU and side-channel attacks)
  memoryCost: 65536,     // 64 MB
  timeCost: 3,           // 3 iterations
  parallelism: 4,        // 4 parallel threads
};

/**
 * Hashes a password using Argon2id with default secure settings.
 */
export async function hashPassword(password: string): Promise<string> {
  if (!password) {
    throw new Error('Password cannot be empty');
  }

  return argon2.hash(password, DEFAULT_OPTIONS);
}

/**
 * Hashes a password using Argon2id with custom options.
 */
export async function hashPasswordWithOptions(
  password: string,
  options: Partial<HashOptions> = {}
): Promise<string> {
  if (!password) {
    throw new Error('Password cannot be empty');
  }

  return argon2.hash(password, { ...DEFAULT_OPTIONS, ...options });
}

/**
 * Verifies that a plaintext password matches an Argon2 hash.
 * Returns true if the password is correct, false otherwise.
 */
export async function verifyPassword(
  password: string,
  hash: string
): Promise<boolean> {
  if (!password || !hash) {
    throw new Error('Password and hash cannot be empty');
  }

  try {
    return await argon2.verify(hash, password);
  } catch (error) {
    // argon2.verify throws on mismatch in some versions
    if (error instanceof Error && error.message.includes('verification failed')) {
      return false;
    }
    throw error;
  }
}
