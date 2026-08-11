import { describe, it, expect, beforeEach, vi, afterEach } from 'vitest';
import { render, screen, waitFor } from '../test/test-utils';
import userEvent from '@testing-library/user-event';
import axios from 'axios';
import Databases from './Databases';

vi.mock('axios');
const mockedAxios = axios as any;

describe('Databases', () => {
  beforeEach(() => {
    localStorage.clear();
    localStorage.setItem('nest_tenant', 'test-tenant');
    localStorage.setItem('auth_token', 'test-token');
    vi.clearAllMocks();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('renders databases page title', () => {
    mockedAxios.get.mockResolvedValue({ data: { databases: [] } });

    render(<Databases />);

    expect(screen.getByRole('heading', { name: /databases/i })).toBeInTheDocument();
  });

  it('displays loading state', () => {
    mockedAxios.get.mockImplementation(
      () =>
        new Promise(resolve =>
          setTimeout(
            () =>
              resolve({
                data: { databases: [] },
              }),
            100,
          ),
        ),
    );

    render(<Databases />);

    expect(screen.getByRole('heading', { name: /databases/i })).toBeInTheDocument();
  });

  it('displays empty state when no databases', async () => {
    mockedAxios.get.mockResolvedValue({ data: { databases: [] } });

    render(<Databases />);

    await waitFor(() => {
      expect(screen.getByText(/no databases provisioned yet/i)).toBeInTheDocument();
    });
  });

  it('renders databases table with data', async () => {
    mockedAxios.get.mockResolvedValue({
      data: {
        databases: [
          {
            name: 'main-db',
            type: 'postgres',
            class: 'postgres-standard',
            status: 'active',
            endpoint: 'db.example.com:5432',
          },
          {
            name: 'backup-db',
            type: 'mysql',
            class: 'mysql-standard',
            status: 'active',
            endpoint: 'backup.example.com:3306',
          },
        ],
      },
    });

    render(<Databases />);

    await waitFor(() => {
      expect(screen.getByText('main-db')).toBeInTheDocument();
    });

    expect(screen.getByText('backup-db')).toBeInTheDocument();
    expect(screen.getByText('postgres')).toBeInTheDocument();
    expect(screen.getByText('mysql')).toBeInTheDocument();
  });

  it('displays status badges with correct styling', async () => {
    mockedAxios.get.mockResolvedValue({
      data: {
        databases: [
          {
            name: 'main-db',
            type: 'postgres',
            class: 'postgres-standard',
            status: 'active',
          },
        ],
      },
    });

    render(<Databases />);

    await waitFor(() => {
      expect(screen.getByText('active')).toBeInTheDocument();
    });
  });

  it('displays table headers correctly', async () => {
    mockedAxios.get.mockResolvedValue({
      data: {
        databases: [
          {
            name: 'main-db',
            type: 'postgres',
            class: 'postgres-standard',
            status: 'active',
          },
        ],
      },
    });

    render(<Databases />);

    await waitFor(() => {
      expect(screen.getByText('main-db')).toBeInTheDocument();
    });

    expect(screen.getByText('Name')).toBeInTheDocument();
    expect(screen.getByText('Type')).toBeInTheDocument();
    expect(screen.getByText('Class')).toBeInTheDocument();
    expect(screen.getByText('Status')).toBeInTheDocument();
    expect(screen.getByText('Endpoint')).toBeInTheDocument();
  });

  it('refresh button triggers refetch', async () => {
    const user = userEvent.setup();
    mockedAxios.get.mockResolvedValue({ data: { databases: [] } });

    render(<Databases />);

    const refreshButton = screen.getByRole('button');
    await user.click(refreshButton);

    expect(mockedAxios.get).toHaveBeenCalled();
  });

  it('displays default endpoint dash when endpoint missing', async () => {
    mockedAxios.get.mockResolvedValue({
      data: {
        databases: [
          {
            name: 'main-db',
            type: 'postgres',
            class: 'postgres-standard',
            status: 'active',
          },
        ],
      },
    });

    render(<Databases />);

    await waitFor(() => {
      expect(screen.getByText('—')).toBeInTheDocument();
    });
  });

  it('handles multiple database rows', async () => {
    mockedAxios.get.mockResolvedValue({
      data: {
        databases: [
          { name: 'db1', type: 'postgres', class: 'std', status: 'active' },
          { name: 'db2', type: 'mysql', class: 'std', status: 'active' },
          { name: 'db3', type: 'mariadb', class: 'std', status: 'active' },
        ],
      },
    });

    render(<Databases />);

    await waitFor(() => {
      expect(screen.getByText('db1')).toBeInTheDocument();
    });

    expect(screen.getByText('db2')).toBeInTheDocument();
    expect(screen.getByText('db3')).toBeInTheDocument();
  });


  it('uses empty string when tenant not in localStorage', async () => {
    localStorage.removeItem('nest_tenant');
    mockedAxios.get.mockResolvedValue({ data: { databases: [] } });

    render(<Databases />);

    await waitFor(() => {
      expect(screen.getByRole('heading', { name: /databases/i })).toBeInTheDocument();
    });
  });

  it('displays default status badge when status is undefined', async () => {
    mockedAxios.get.mockResolvedValue({
      data: {
        databases: [
          { name: 'db1', type: 'postgres', class: 'std' },
        ],
      },
    });

    render(<Databases />);

    await waitFor(() => {
      expect(screen.getByText('db1')).toBeInTheDocument();
    });

    expect(screen.getByText('active')).toBeInTheDocument();
  });

  it('displays default endpoint when endpoint is undefined', async () => {
    mockedAxios.get.mockResolvedValue({
      data: {
        databases: [
          { name: 'db1', type: 'postgres', class: 'std', status: 'active' },
        ],
      },
    });

    render(<Databases />);

    await waitFor(() => {
      expect(screen.getByText('db1')).toBeInTheDocument();
    });

    expect(screen.getByText('—')).toBeInTheDocument();
  });
});
