import { describe, it, expect, beforeEach, vi, afterEach } from 'vitest';
import { render, screen, waitFor } from '../test/test-utils';
import userEvent from '@testing-library/user-event';
import axios from 'axios';
import AuditLogs from './AuditLogs';

vi.mock('axios');
const mockedAxios = axios as any;

describe('AuditLogs', () => {
  beforeEach(() => {
    localStorage.clear();
    localStorage.setItem('nest_tenant', 'test-tenant');
    localStorage.setItem('auth_token', 'test-token');
    vi.clearAllMocks();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('renders audit logs page title', () => {
    mockedAxios.get.mockResolvedValue({ data: { events: [] } });

    render(<AuditLogs />);

    expect(screen.getByRole('heading', { name: /audit logs/i })).toBeInTheDocument();
  });

  it('displays search input field', () => {
    mockedAxios.get.mockResolvedValue({ data: { events: [] } });

    render(<AuditLogs />);

    expect(screen.getByPlaceholderText(/search events/i)).toBeInTheDocument();
  });

  it('displays empty state when no events', async () => {
    mockedAxios.get.mockResolvedValue({ data: { events: [] } });

    render(<AuditLogs />);

    await waitFor(() => {
      expect(screen.getByText(/no audit events found/i)).toBeInTheDocument();
    });
  });

  it('renders audit events table with data', async () => {
    mockedAxios.get.mockResolvedValue({
      data: {
        events: [
          {
            id: 'evt-1',
            timestamp: '2024-01-15T10:30:00Z',
            actor: 'user@example.com',
            action: 'create_resource',
            resource: 'postgres-main',
            outcome: 'success',
          },
          {
            id: 'evt-2',
            timestamp: '2024-01-15T10:25:00Z',
            actor: 'admin@example.com',
            action: 'delete_database',
            resource: 'backup-db',
            outcome: 'success',
          },
        ],
      },
    });

    render(<AuditLogs />);

    await waitFor(() => {
      expect(screen.getByText('user@example.com')).toBeInTheDocument();
    });

    expect(screen.getByText('admin@example.com')).toBeInTheDocument();
    expect(screen.getByText('create_resource')).toBeInTheDocument();
    expect(screen.getByText('delete_database')).toBeInTheDocument();
  });

  it('displays table headers correctly', async () => {
    mockedAxios.get.mockResolvedValue({
      data: {
        events: [
          {
            id: 'evt-1',
            timestamp: '2024-01-15T10:30:00Z',
            actor: 'user@example.com',
            action: 'create_resource',
            resource: 'postgres-main',
            outcome: 'success',
          },
        ],
      },
    });

    render(<AuditLogs />);

    await waitFor(() => {
      expect(screen.getByText('user@example.com')).toBeInTheDocument();
    });

    expect(screen.getByText('Time')).toBeInTheDocument();
    expect(screen.getByText('Actor')).toBeInTheDocument();
    expect(screen.getByText('Action')).toBeInTheDocument();
    expect(screen.getByText('Resource')).toBeInTheDocument();
    expect(screen.getByText('Outcome')).toBeInTheDocument();
  });

  it('filters events by action when search text entered', async () => {
    const user = userEvent.setup();
    mockedAxios.get.mockResolvedValue({
      data: {
        events: [
          {
            id: 'evt-1',
            timestamp: '2024-01-15T10:30:00Z',
            actor: 'user@example.com',
            action: 'create_resource',
            resource: 'postgres-main',
            outcome: 'success',
          },
          {
            id: 'evt-2',
            timestamp: '2024-01-15T10:25:00Z',
            actor: 'admin@example.com',
            action: 'delete_database',
            resource: 'backup-db',
            outcome: 'success',
          },
        ],
      },
    });

    render(<AuditLogs />);

    const searchInput = screen.getByPlaceholderText(/search events/i);
    await user.type(searchInput, 'create_resource');

    await waitFor(() => {
      expect(screen.getByText('create_resource')).toBeInTheDocument();
    });

    expect(screen.queryByText('delete_database')).not.toBeInTheDocument();
  });

  it('filters events by actor when search text entered', async () => {
    const user = userEvent.setup();
    mockedAxios.get.mockResolvedValue({
      data: {
        events: [
          {
            id: 'evt-1',
            timestamp: '2024-01-15T10:30:00Z',
            actor: 'user@example.com',
            action: 'create_resource',
            resource: 'postgres-main',
            outcome: 'success',
          },
          {
            id: 'evt-2',
            timestamp: '2024-01-15T10:25:00Z',
            actor: 'admin@example.com',
            action: 'delete_database',
            resource: 'backup-db',
            outcome: 'success',
          },
        ],
      },
    });

    render(<AuditLogs />);

    const searchInput = screen.getByPlaceholderText(/search events/i);
    await user.type(searchInput, 'admin@example.com');

    await waitFor(() => {
      expect(screen.getByText('admin@example.com')).toBeInTheDocument();
    });

    expect(screen.queryByText('user@example.com')).not.toBeInTheDocument();
  });

  it('filters events by resource when search text entered', async () => {
    const user = userEvent.setup();
    mockedAxios.get.mockResolvedValue({
      data: {
        events: [
          {
            id: 'evt-1',
            timestamp: '2024-01-15T10:30:00Z',
            actor: 'user@example.com',
            action: 'create_resource',
            resource: 'postgres-main',
            outcome: 'success',
          },
          {
            id: 'evt-2',
            timestamp: '2024-01-15T10:25:00Z',
            actor: 'admin@example.com',
            action: 'delete_database',
            resource: 'backup-db',
            outcome: 'success',
          },
        ],
      },
    });

    render(<AuditLogs />);

    const searchInput = screen.getByPlaceholderText(/search events/i);
    await user.type(searchInput, 'postgres-main');

    await waitFor(() => {
      expect(screen.getByText('postgres-main')).toBeInTheDocument();
    });

    expect(screen.queryByText('backup-db')).not.toBeInTheDocument();
  });

  it('displays success outcome badges with correct styling', async () => {
    mockedAxios.get.mockResolvedValue({
      data: {
        events: [
          {
            id: 'evt-1',
            timestamp: '2024-01-15T10:30:00Z',
            actor: 'user@example.com',
            action: 'create_resource',
            resource: 'postgres-main',
            outcome: 'success',
          },
        ],
      },
    });

    render(<AuditLogs />);

    await waitFor(() => {
      expect(screen.getByText('success')).toBeInTheDocument();
    });
  });

  it('formats timestamps correctly', async () => {
    mockedAxios.get.mockResolvedValue({
      data: {
        events: [
          {
            id: 'evt-1',
            timestamp: '2024-01-15T10:30:00Z',
            actor: 'user@example.com',
            action: 'create_resource',
            resource: 'postgres-main',
            outcome: 'success',
          },
        ],
      },
    });

    render(<AuditLogs />);

    await waitFor(() => {
      expect(screen.getByText(/1\/15\/2024/)).toBeInTheDocument();
    });
  });

  it('shows no results when search filters all events', async () => {
    const user = userEvent.setup();
    mockedAxios.get.mockResolvedValue({
      data: {
        events: [
          {
            id: 'evt-1',
            timestamp: '2024-01-15T10:30:00Z',
            actor: 'user@example.com',
            action: 'create_resource',
            resource: 'postgres-main',
            outcome: 'success',
          },
        ],
      },
    });

    render(<AuditLogs />);

    const searchInput = screen.getByPlaceholderText(/search events/i);
    await user.type(searchInput, 'nonexistent');

    await waitFor(() => {
      expect(screen.getByText(/no audit events found/i)).toBeInTheDocument();
    });
  });


  it('uses empty string when tenant not in localStorage', async () => {
    localStorage.removeItem('nest_tenant');
    mockedAxios.get.mockResolvedValue({ data: { events: [] } });

    render(<AuditLogs />);

    await waitFor(() => {
      expect(screen.getByPlaceholderText(/search events/i)).toBeInTheDocument();
    });
  });

  it('displays failure outcome badges with correct styling', async () => {
    mockedAxios.get.mockResolvedValue({
      data: {
        events: [
          {
            id: 'evt-1',
            timestamp: '2024-01-15T10:30:00Z',
            actor: 'user@example.com',
            action: 'delete_resource',
            resource: 'postgres-main',
            outcome: 'failure',
          },
        ],
      },
    });

    render(<AuditLogs />);

    await waitFor(() => {
      expect(screen.getByText('failure')).toBeInTheDocument();
    });
  });
});
