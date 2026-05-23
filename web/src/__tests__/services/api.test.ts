/**
 * Unit tests for the API client (axios instance)
 */

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import axios from 'axios';

// Mock axios.create to return a controllable instance
vi.mock('axios', async () => {
  const interceptors = {
    request: { use: vi.fn(), eject: vi.fn() },
    response: { use: vi.fn(), eject: vi.fn() },
  };
  const instance = {
    interceptors,
    get: vi.fn(),
    post: vi.fn(),
    defaults: { headers: { common: {} } },
  };
  return {
    default: {
      create: vi.fn(() => instance),
    },
  };
});

describe('API client', () => {
  let requestInterceptor: (config: Record<string, unknown>) => Record<string, unknown>;
  let responseErrorInterceptor: (error: Record<string, unknown>) => Promise<never>;

  beforeEach(async () => {
    vi.resetModules();
    vi.stubGlobal('localStorage', {
      getItem: vi.fn(),
      setItem: vi.fn(),
      removeItem: vi.fn(),
    });

    // Re-import to trigger module execution and capture interceptors
    await import('../../services/api');

    const createdInstance = (axios.create as ReturnType<typeof vi.fn>).mock.results[0]?.value;
    const reqUse = createdInstance.interceptors.request.use as ReturnType<typeof vi.fn>;
    const resUse = createdInstance.interceptors.response.use as ReturnType<typeof vi.fn>;

    requestInterceptor = reqUse.mock.calls[0]?.[0];
    responseErrorInterceptor = resUse.mock.calls[0]?.[1];
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  describe('instance creation', () => {
    it('should create axios instance with correct baseURL', () => {
      expect(axios.create).toHaveBeenCalledWith(
        expect.objectContaining({
          baseURL: expect.stringContaining('/api/v1'),
        }),
      );
    });

    it('should set Content-Type to application/json', () => {
      expect(axios.create).toHaveBeenCalledWith(
        expect.objectContaining({
          headers: expect.objectContaining({
            'Content-Type': 'application/json',
          }),
        }),
      );
    });
  });

  describe('request interceptor', () => {
    it('should add Bearer token when auth_token exists', () => {
      (localStorage.getItem as ReturnType<typeof vi.fn>).mockReturnValue('test-token-123');
      const config = { headers: {} } as Record<string, Record<string, string>>;
      const result = requestInterceptor(config) as typeof config;
      expect(result.headers.Authorization).toBe('Bearer test-token-123');
    });

    it('should not add Authorization header when no token', () => {
      (localStorage.getItem as ReturnType<typeof vi.fn>).mockReturnValue(null);
      const config = { headers: {} } as Record<string, Record<string, string>>;
      const result = requestInterceptor(config) as typeof config;
      expect(result.headers.Authorization).toBeUndefined();
    });
  });

  describe('response interceptor (error)', () => {
    it('should remove token and redirect on 401', async () => {
      const originalHref = window.location.href;
      // Mock window.location
      Object.defineProperty(window, 'location', {
        value: { href: originalHref },
        writable: true,
        configurable: true,
      });

      const error = { response: { status: 401 } };
      await expect(responseErrorInterceptor(error)).rejects.toBeDefined();
      expect(localStorage.removeItem).toHaveBeenCalledWith('auth_token');
      expect(window.location.href).toBe('/login');
    });

    it('should reject without redirect on non-401 errors', async () => {
      const error = { response: { status: 500 } };
      await expect(responseErrorInterceptor(error)).rejects.toBeDefined();
      expect(localStorage.removeItem).not.toHaveBeenCalled();
    });
  });
});
