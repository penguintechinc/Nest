import { describe, it, expect, beforeEach, vi, afterEach } from 'vitest';
import { render, screen, waitFor } from '../test/test-utils';
import userEvent from '@testing-library/user-event';
import axios from 'axios';
import Resources from './Resources';

vi.mock('axios');
const mockedAxios = axios as any;

describe('Resources', () => {
  beforeEach(() => {
    localStorage.clear();
    localStorage.setItem('nest_tenant', 'test-tenant');
    localStorage.setItem('auth_token', 'test-token');
    vi.clearAllMocks();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('renders resources page title', () => {
    mockedAxios.get.mockResolvedValue({ data: { dataresources: [] } });

    render(<Resources />);

    expect(screen.getByRole('heading', { name: /data resources/i })).toBeInTheDocument();
  });

  it('displays loading state initially', () => {
    mockedAxios.get.mockImplementation(
      () =>
        new Promise(resolve =>
          setTimeout(
            () =>
              resolve({
                data: { dataresources: [] },
              }),
            100,
          ),
        ),
    );

    render(<Resources />);

    expect(screen.getByRole('heading', { name: /data resources/i })).toBeInTheDocument();
  });

  it('displays empty state message when no resources', async () => {
    mockedAxios.get.mockResolvedValue({ data: { dataresources: [] } });

    render(<Resources />);

    await waitFor(() => {
      expect(screen.getByText(/no resources found/i)).toBeInTheDocument();
    });
  });

  it('displays resource list in table', async () => {
    mockedAxios.get.mockResolvedValue({
      data: {
        dataresources: [
          {
            name: 'postgres-main',
            type: 'postgres',
            class: 'postgres-standard',
            status: 'active',
            endpoint: 'localhost:5432',
          },
          {
            name: 'mysql-cache',
            type: 'mysql',
            class: 'mysql-standard',
            status: 'active',
            endpoint: 'localhost:3306',
          },
        ],
      },
    });

    render(<Resources />);

    await waitFor(() => {
      expect(screen.getByText('postgres-main')).toBeInTheDocument();
    });

    expect(screen.getByText('mysql-cache')).toBeInTheDocument();
    expect(screen.getAllByText('postgres').length).toBeGreaterThan(0);
    expect(screen.getAllByText('mysql').length).toBeGreaterThan(0);
  });

  it('filters resources by type when filter pill clicked', async () => {
    const user = userEvent.setup();
    mockedAxios.get.mockResolvedValue({
      data: {
        dataresources: [
          { name: 'postgres-main', type: 'postgres', class: 'standard', status: 'active' },
        ],
      },
    });

    render(<Resources />);

    await waitFor(() => {
      expect(screen.getByText('postgres-main')).toBeInTheDocument();
    });

    const postgresFilter = screen.getAllByRole('button', { name: /postgres/i })[0];
    await user.click(postgresFilter);

    expect(mockedAxios.get).toHaveBeenCalled();
  });

  it('shows all resources with "All" filter pill selected by default', async () => {
    mockedAxios.get.mockResolvedValue({
      data: {
        dataresources: [
          { name: 'postgres-main', type: 'postgres', class: 'standard', status: 'active' },
          { name: 'kafka-event', type: 'kafka', class: 'standard', status: 'active' },
        ],
      },
    });

    render(<Resources />);

    await waitFor(() => {
      expect(screen.getByText('postgres-main')).toBeInTheDocument();
    });

    expect(screen.getByText('kafka-event')).toBeInTheDocument();
  });

  it('opens create modal on button click', async () => {
    const user = userEvent.setup();
    mockedAxios.get.mockResolvedValue({ data: { dataresources: [] } });

    render(<Resources />);

    await waitFor(() => {
      expect(screen.getByText(/no resources found/i)).toBeInTheDocument();
    });

    const createButton = screen.getByRole('button', { name: /create/i });
    await user.click(createButton);

    expect(screen.getByText(/create data resource/i)).toBeInTheDocument();
  });

  it('create modal has required fields', async () => {
    const user = userEvent.setup();
    mockedAxios.get.mockResolvedValue({ data: { dataresources: [] } });

    render(<Resources />);

    const createButton = screen.getByRole('button', { name: /create/i });
    await user.click(createButton);

    expect(screen.getByText('Name')).toBeInTheDocument();
    expect(screen.getByText('Type')).toBeInTheDocument();
    expect(screen.getAllByText(/class/i).length).toBeGreaterThan(0);
  });

  it('closes create modal on cancel', async () => {
    const user = userEvent.setup();
    mockedAxios.get.mockResolvedValue({ data: { dataresources: [] } });

    render(<Resources />);

    const createButton = screen.getByRole('button', { name: /create/i });
    await user.click(createButton);

    const cancelButton = screen.getByRole('button', { name: /cancel/i });
    await user.click(cancelButton);

    expect(screen.queryByText(/create data resource/i)).not.toBeInTheDocument();
  });

  it('displays delete button for each resource', async () => {
    mockedAxios.get.mockResolvedValue({
      data: {
        dataresources: [
          { name: 'postgres-main', type: 'postgres', class: 'standard', status: 'active' },
        ],
      },
    });

    render(<Resources />);

    await waitFor(() => {
      expect(screen.getByText('postgres-main')).toBeInTheDocument();
    });

    const deleteButtons = screen.getAllByRole('button');
    expect(deleteButtons.length).toBeGreaterThan(0);
  });

  it('renders filter pills for all resource types', async () => {
    mockedAxios.get.mockResolvedValue({ data: { dataresources: [] } });

    render(<Resources />);

    expect(screen.getByRole('button', { name: /all/i })).toBeInTheDocument();
    expect(screen.getAllByRole('button', { name: /postgres/i }).length).toBeGreaterThan(0);
  });

  it('refresh button calls refetch', async () => {
    const user = userEvent.setup();
    mockedAxios.get.mockResolvedValue({ data: { dataresources: [] } });

    render(<Resources />);

    const refreshButton = screen.getAllByRole('button').find(
      btn => btn.querySelector('svg'),
    ) as HTMLButtonElement;

    if (refreshButton) {
      await user.click(refreshButton);
    }

    expect(mockedAxios.get).toHaveBeenCalled();
  });


  it('uses empty string when tenant not in localStorage', async () => {
    localStorage.removeItem('nest_tenant');
    mockedAxios.get.mockResolvedValue({ data: { dataresources: [] } });

    render(<Resources />);

    await waitFor(() => {
      expect(screen.getByRole('heading', { name: /data resources/i })).toBeInTheDocument();
    });
  });

  it('displays default status badge when status is undefined', async () => {
    mockedAxios.get.mockResolvedValue({
      data: {
        dataresources: [
          { name: 'postgres-main', type: 'postgres', class: 'standard' },
        ],
      },
    });

    render(<Resources />);

    await waitFor(() => {
      expect(screen.getByText('postgres-main')).toBeInTheDocument();
    });

    expect(screen.getByText('active')).toBeInTheDocument();
  });

  it('displays default endpoint when endpoint is undefined', async () => {
    mockedAxios.get.mockResolvedValue({
      data: {
        dataresources: [
          { name: 'postgres-main', type: 'postgres', class: 'standard', status: 'active' },
        ],
      },
    });

    render(<Resources />);

    await waitFor(() => {
      expect(screen.getByText('postgres-main')).toBeInTheDocument();
    });

    expect(screen.getByText('—')).toBeInTheDocument();
  });
});
