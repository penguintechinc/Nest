/**
 * Unit tests for Zustand auth store
 */

import { describe, it, expect, vi, beforeEach } from 'vitest';
import { act } from '@testing-library/react';

// Mock auth service before importing store
vi.mock('../../services/auth', () => {
  return {
    __esModule: true,
    default: {
      logout: vi.fn(),
      isAuthenticated: vi.fn(),
      getCurrentUser: vi.fn(),
    },
  };
});

import useAuthStore from '../../stores/authStore';
import authService from '../../services/auth';

const mockedAuthService = vi.mocked(authService);

describe('useAuthStore', () => {
  beforeEach(() => {
    // Reset store state between tests
    act(() => {
      useAuthStore.setState({
        user: null,
        isLoading: false,
        error: null,
        isAuthenticated: false,
      });
    });
    vi.clearAllMocks();
  });

  describe('initial state', () => {
    it('should have user as null', () => {
      const state = useAuthStore.getState();
      expect(state.user).toBeNull();
    });

    it('should have isAuthenticated as false', () => {
      const state = useAuthStore.getState();
      expect(state.isAuthenticated).toBe(false);
    });

    it('should have isLoading as false', () => {
      const state = useAuthStore.getState();
      expect(state.isLoading).toBe(false);
    });

    it('should have error as null', () => {
      const state = useAuthStore.getState();
      expect(state.error).toBeNull();
    });
  });

  describe('setUser', () => {
    it('should set user and mark as authenticated', () => {
      const mockUser = { id: '1', email: 'test@example.com', name: 'Test' };
      act(() => {
        useAuthStore.getState().setUser(mockUser);
      });
      const state = useAuthStore.getState();
      expect(state.user).toEqual(mockUser);
      expect(state.isAuthenticated).toBe(true);
      expect(state.error).toBeNull();
    });

    it('should clear authentication when user is null', () => {
      act(() => {
        useAuthStore.getState().setUser(null);
      });
      const state = useAuthStore.getState();
      expect(state.user).toBeNull();
      expect(state.isAuthenticated).toBe(false);
    });
  });

  describe('logout', () => {
    it('should clear user, auth state, and call authService.logout', () => {
      const mockUser = { id: '1', email: 'test@example.com', name: 'Test' };
      act(() => {
        useAuthStore.getState().setUser(mockUser);
      });
      act(() => {
        useAuthStore.getState().logout();
      });
      const state = useAuthStore.getState();
      expect(state.user).toBeNull();
      expect(state.isAuthenticated).toBe(false);
      expect(state.error).toBeNull();
      expect(mockedAuthService.logout).toHaveBeenCalledOnce();
    });
  });

  describe('checkAuth', () => {
    it('should set user when token is valid', async () => {
      const mockUser = { id: '1', email: 'test@example.com', name: 'Test' };
      mockedAuthService.isAuthenticated.mockReturnValue(true);
      mockedAuthService.getCurrentUser.mockResolvedValue(mockUser);

      await act(async () => {
        await useAuthStore.getState().checkAuth();
      });

      const state = useAuthStore.getState();
      expect(state.user).toEqual(mockUser);
      expect(state.isAuthenticated).toBe(true);
      expect(state.isLoading).toBe(false);
    });

    it('should clear user when no token', async () => {
      mockedAuthService.isAuthenticated.mockReturnValue(false);

      await act(async () => {
        await useAuthStore.getState().checkAuth();
      });

      const state = useAuthStore.getState();
      expect(state.user).toBeNull();
      expect(state.isAuthenticated).toBe(false);
      expect(state.isLoading).toBe(false);
    });

    it('should set error when getCurrentUser fails', async () => {
      mockedAuthService.isAuthenticated.mockReturnValue(true);
      mockedAuthService.getCurrentUser.mockRejectedValue(new Error('Network error'));

      await act(async () => {
        await useAuthStore.getState().checkAuth();
      });

      const state = useAuthStore.getState();
      expect(state.user).toBeNull();
      expect(state.isAuthenticated).toBe(false);
      expect(state.error).toBe('Failed to verify authentication');
      expect(state.isLoading).toBe(false);
    });
  });
});
