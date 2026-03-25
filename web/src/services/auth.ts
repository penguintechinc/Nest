/**
 * Authentication service
 * Handles login, logout, and token management
 */

import api from './api';

export interface LoginRequest {
  email: string;
  password: string;
}

export interface LoginResponse {
  token: string;
  user: {
    id: string;
    email: string;
    name: string;
  };
}

export interface User {
  id: string;
  email: string;
  name: string;
}

class AuthService {
  /**
   * Login with email and password
   */
  async login(credentials: LoginRequest): Promise<LoginResponse> {
    try {
      const response = await api.post<LoginResponse>('/auth/login', credentials);
      const { token } = response.data;
      localStorage.setItem('auth_token', token);
      return response.data;
    } catch (_error) {
      throw new Error('Login failed: Invalid credentials');
    }
  }

  /**
   * Register new user
   */
  async register(data: { email: string; password: string; name: string }): Promise<LoginResponse> {
    try {
      const response = await api.post<LoginResponse>('/auth/register', data);
      const { token } = response.data;
      localStorage.setItem('auth_token', token);
      return response.data;
    } catch (_error) {
      throw new Error('Registration failed');
    }
  }

  /**
   * Logout user
   */
  logout(): void {
    localStorage.removeItem('auth_token');
    window.location.href = '/login';
  }

  /**
   * Get current user
   */
  async getCurrentUser(): Promise<User> {
    try {
      const response = await api.get<User>('/auth/me');
      return response.data;
    } catch (_error) {
      throw new Error('Failed to fetch current user');
    }
  }

  /**
   * Check if user is authenticated
   */
  isAuthenticated(): boolean {
    return !!localStorage.getItem('auth_token');
  }

  /**
   * Get stored token
   */
  getToken(): string | null {
    return localStorage.getItem('auth_token');
  }
}

export default new AuthService();
