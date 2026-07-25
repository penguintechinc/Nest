import { describe, it, expect, beforeEach, vi, afterEach } from 'vitest';
import { render, screen, waitFor } from '../test/test-utils';
import axios from 'axios';
import Dashboard from './Dashboard';

vi.mock('axios');
const mockedAxios = axios as any;

describe('Dashboard', () => {
  beforeEach(() => {
    localStorage.clear();
    localStorage.setItem('nest_tenant', 'test-tenant');
    localStorage.setItem('auth_token', 'test-token');
    vi.clearAllMocks();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('renders dashboard title and tenant info', () => {
    mockedAxios.get.mockResolvedValue({ data: { dataresources: [], databases: [] } });

    render(<Dashboard />);

    expect(screen.getByRole('heading', { name: /dashboard/i })).toBeInTheDocument();
    expect(screen.getByText(/tenant:/i)).toBeInTheDocument();
    expect(screen.getByText('test-tenant')).toBeInTheDocument();
  });

  it('renders stat cards with correct labels', async () => {
    mockedAxios.get.mockImplementation((url: string) => {
      if (url.includes('dataresources')) {
        return Promise.resolve({
          data: {
            dataresources: [{ name: 'res1', type: 'postgres', status: 'active' }],
          },
        });
      }
      if (url.includes('databases')) {
        return Promise.resolve({
          data: {
            databases: [{ name: 'db1', type: 'postgres', status: 'active' }],
          },
        });
      }
      return Promise.resolve({ data: {} });
    });

    render(<Dashboard />);

    await waitFor(() => {
      expect(screen.getByText(/data resources/i)).toBeInTheDocument();
    });

    expect(screen.getByText(/databases/i)).toBeInTheDocument();
    expect(screen.getByText(/storage/i)).toBeInTheDocument();
    expect(screen.getByText(/status/i)).toBeInTheDocument();
  });

  it('displays resource count from API', async () => {
    mockedAxios.get.mockImplementation((url: string) => {
      if (url.includes('dataresources')) {
        return Promise.resolve({
          data: {
            dataresources: [
              { name: 'res1', type: 'postgres', status: 'active' },
              { name: 'res2', type: 'mysql', status: 'active' },
            ],
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

    render(<Dashboard />);

    await waitFor(() => {
      expect(screen.getByText('2')).toBeInTheDocument();
    });
  });

  it('displays database count from API', async () => {
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
            databases: [
              { name: 'db1', type: 'postgres', status: 'active' },
              { name: 'db2', type: 'mysql', status: 'active' },
              { name: 'db3', type: 'mariadb', status: 'active' },
            ],
          },
        });
      }
      return Promise.resolve({ data: {} });
    });

    render(<Dashboard />);

    await waitFor(() => {
      expect(screen.getByText('3')).toBeInTheDocument();
    });
  });

  it('displays recent resources table when data available', async () => {
    mockedAxios.get.mockImplementation((url: string) => {
      if (url.includes('dataresources')) {
        return Promise.resolve({
          data: {
            dataresources: [
              { name: 'postgres-prod', type: 'postgres', status: 'active' },
              { name: 'mysql-staging', type: 'mysql', status: 'active' },
            ],
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

    render(<Dashboard />);

    await waitFor(() => {
      expect(screen.getByText('postgres-prod')).toBeInTheDocument();
    });

    expect(screen.getByText('mysql-staging')).toBeInTheDocument();
    expect(screen.getByText('postgres')).toBeInTheDocument();
  });

  it('displays empty state message when no resources', async () => {
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

    render(<Dashboard />);

    await waitFor(() => {
      expect(screen.getByText(/no resources yet/i)).toBeInTheDocument();
    });
  });

  it('shows default values for missing stats', async () => {
    mockedAxios.get.mockResolvedValue({ data: {} });

    render(<Dashboard />);

    await waitFor(() => {
      expect(screen.getAllByText('0')[0]).toBeInTheDocument();
    });
    expect(screen.getByText('Healthy')).toBeInTheDocument();
  });


  it('uses empty string when tenant not in localStorage', async () => {
    localStorage.removeItem('nest_tenant');
    mockedAxios.get.mockResolvedValue({ data: { dataresources: [], databases: [] } });

    render(<Dashboard />);

    await waitFor(() => {
      expect(screen.getByRole('heading', { name: /dashboard/i })).toBeInTheDocument();
    });
  });

  it('displays default status badge when status is undefined', async () => {
    mockedAxios.get.mockImplementation((url: string) => {
      if (url.includes('dataresources')) {
        return Promise.resolve({
          data: {
            dataresources: [{ name: 'res1', type: 'postgres' }],
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

    render(<Dashboard />);

    await waitFor(() => {
      expect(screen.getByText('active')).toBeInTheDocument();
    });
  });
});
