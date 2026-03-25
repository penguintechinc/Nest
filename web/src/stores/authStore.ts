/**
 * Zustand authentication store
 * Global state management for authentication
 */

import { create } from 'zustand';
import { User } from '../services/auth';
import authService from '../services/auth';

interface AuthState {
  user: User | null;
  isLoading: boolean;
  error: string | null;
  isAuthenticated: boolean;

  // Actions
  setUser: (user: User | null) => void;
  setLoading: (loading: boolean) => void;
  setError: (error: string | null) => void;
  logout: () => void;
  checkAuth: () => Promise<void>;
}

const useAuthStore = create<AuthState>((set) => ({
  user: null,
  isLoading: false,
  error: null,
  isAuthenticated: false,

  setUser: (user) => {
    set({
      user,
      isAuthenticated: user !== null,
      error: null,
    });
  },

  setLoading: (isLoading) => {
    set({ isLoading });
  },

  setError: (error) => {
    set({ error });
  },

  logout: () => {
    set({
      user: null,
      isAuthenticated: false,
      error: null,
    });
    authService.logout();
  },

  checkAuth: async () => {
    set({ isLoading: true });
    try {
      if (authService.isAuthenticated()) {
        const user = await authService.getCurrentUser();
        set({
          user,
          isAuthenticated: true,
          isLoading: false,
        });
      } else {
        set({
          user: null,
          isAuthenticated: false,
          isLoading: false,
        });
      }
    } catch (_error) {
      set({
        user: null,
        isAuthenticated: false,
        error: 'Failed to verify authentication',
        isLoading: false,
      });
    }
  },
}));

export default useAuthStore;
