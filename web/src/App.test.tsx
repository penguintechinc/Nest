import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render, screen, waitFor } from './test/test-utils';
import axios from 'axios';
import App from './App';

vi.mock('axios');
const mockedAxios = axios as any;

describe('App', () => {
  beforeEach(() => {
    localStorage.clear();
    vi.clearAllMocks();
  });

  it('renders login page when no token in localStorage', () => {
    render(<App />);

    expect(screen.getByLabelText(/tenant/i)).toBeInTheDocument();
    expect(screen.getByLabelText(/email/i)).toBeInTheDocument();
    expect(screen.getByAltText(/nest logo/i)).toBeInTheDocument();
  });

  it('renders layout with dashboard when token exists', async () => {
    localStorage.setItem('auth_token', 'test-token');
    localStorage.setItem('nest_tenant', 'test-tenant');

    mockedAxios.get.mockImplementation((url: string) => {
      if (url.includes('dataresources')) {
        return Promise.resolve({
          data: {
            dataresources: [],
          },
        });
      }
      if (url.includes('databases')) {
        return Promise.resolve({
          data: {
            databases: [],
          },
        });
      }
      return Promise.resolve({ data: {} });
    });

    render(<App />);

    await waitFor(() => {
      expect(screen.getByAltText('Nest')).toBeInTheDocument();
    });

    expect(screen.getAllByText(/dashboard/i).length).toBeGreaterThan(0);
    expect(screen.getAllByText(/resources/i).length).toBeGreaterThan(0);
  });

  it('shows login page when token removed', () => {
    localStorage.clear();
    render(<App />);

    expect(screen.getByLabelText(/tenant/i)).toBeInTheDocument();
    expect(screen.getByLabelText(/email/i)).toBeInTheDocument();
  });

  it('redirects non-existent routes to login when not authenticated', () => {
    render(<App />);

    expect(screen.getByLabelText(/tenant/i)).toBeInTheDocument();
  });

  it('redirects root path to dashboard when authenticated', async () => {
    localStorage.setItem('auth_token', 'test-token');
    localStorage.setItem('nest_tenant', 'test-tenant');

    mockedAxios.get.mockImplementation((url: string) => {
      if (url.includes('dataresources')) {
        return Promise.resolve({
          data: {
            dataresources: [],
          },
        });
      }
      if (url.includes('databases')) {
        return Promise.resolve({
          data: {
            databases: [],
          },
        });
      }
      return Promise.resolve({ data: {} });
    });

    render(<App />);

    await waitFor(() => {
      expect(screen.getByRole('heading', { name: /dashboard/i })).toBeInTheDocument();
    });
  });

  it('has login route available', () => {
    render(<App />);

    expect(screen.getByLabelText(/tenant/i)).toBeInTheDocument();
  });
});
