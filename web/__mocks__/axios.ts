import { vi } from 'vitest';

/**
 * Manual mock for axios used by `vi.mock('axios')` in component/page specs.
 *
 * The app builds its client with `axios.create()` (see src/services/api.ts) and
 * calls `.interceptors.request.use(...)` at module load. The default auto-mock
 * makes `create()` return undefined, so that call throws on import. Here,
 * `create()` returns the mock itself, so a spec that stubs `axios.get(...)` and
 * the `api = axios.create()` instance observe the same mock functions.
 */
const mockAxios: Record<string, unknown> = {
  get: vi.fn(() => Promise.resolve({ data: {} })),
  post: vi.fn(() => Promise.resolve({ data: {} })),
  put: vi.fn(() => Promise.resolve({ data: {} })),
  patch: vi.fn(() => Promise.resolve({ data: {} })),
  delete: vi.fn(() => Promise.resolve({ data: {} })),
  request: vi.fn(() => Promise.resolve({ data: {} })),
  interceptors: {
    request: { use: vi.fn(), eject: vi.fn() },
    response: { use: vi.fn(), eject: vi.fn() },
  },
  defaults: { headers: { common: {} } },
};

// create() yields the same instance so instance and default-export calls align.
mockAxios.create = vi.fn(() => mockAxios);

export default mockAxios;
