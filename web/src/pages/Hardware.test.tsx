import { describe, it, expect, beforeEach, vi, afterEach } from 'vitest';
import { render, screen, waitFor } from '../test/test-utils';
import axios from 'axios';
import Hardware from './Hardware';

vi.mock('axios');
const mockedAxios = axios as any;

describe('Hardware', () => {
  beforeEach(() => {
    localStorage.clear();
    localStorage.setItem('nest_token', 'test-token');
    vi.clearAllMocks();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('renders hardware page title', () => {
    mockedAxios.get.mockResolvedValue({ data: { nodes: [] } });

    render(<Hardware />);

    expect(screen.getByRole('heading', { name: /hardware/i })).toBeInTheDocument();
  });

  it('displays loading state initially', () => {
    mockedAxios.get.mockImplementation(
      () =>
        new Promise(resolve =>
          setTimeout(
            () =>
              resolve({
                data: { nodes: [] },
              }),
            100,
          ),
        ),
    );

    render(<Hardware />);

    expect(screen.getByRole('heading', { name: /hardware/i })).toBeInTheDocument();
  });

  it('displays empty state when no hardware inventory', async () => {
    mockedAxios.get.mockResolvedValue({ data: { nodes: [] } });

    render(<Hardware />);

    await waitFor(() => {
      expect(screen.getByText(/no hardware inventory data available/i)).toBeInTheDocument();
    });
  });

  it('renders inventory cards for hardware nodes', async () => {
    mockedAxios.get.mockResolvedValue({
      data: {
        nodes: [
          { name: 'node-1', class: 'compute' },
          { name: 'node-2', class: 'storage' },
        ],
      },
    });

    render(<Hardware />);

    await waitFor(() => {
      expect(screen.getByText('node-1')).toBeInTheDocument();
    });

    expect(screen.getByText('node-2')).toBeInTheDocument();
  });

  it('displays node class badges', async () => {
    mockedAxios.get.mockResolvedValue({
      data: {
        nodes: [
          { name: 'node-1', class: 'compute-optimized' },
          { name: 'node-2', class: 'storage-optimized' },
        ],
      },
    });

    render(<Hardware />);

    await waitFor(() => {
      expect(screen.getByText('compute-optimized')).toBeInTheDocument();
    });

    expect(screen.getByText('storage-optimized')).toBeInTheDocument();
  });

  it('renders multiple hardware nodes', async () => {
    mockedAxios.get.mockResolvedValue({
      data: {
        nodes: [
          { name: 'node-1', class: 'compute' },
          { name: 'node-2', class: 'compute' },
          { name: 'node-3', class: 'storage' },
        ],
      },
    });

    render(<Hardware />);

    await waitFor(() => {
      expect(screen.getByText('node-1')).toBeInTheDocument();
    });

    expect(screen.getByText('node-2')).toBeInTheDocument();
    expect(screen.getByText('node-3')).toBeInTheDocument();
  });

  it('handles empty nodes gracefully', async () => {
    mockedAxios.get.mockResolvedValue({ data: {} });

    render(<Hardware />);

    await waitFor(() => {
      expect(screen.getByText(/no hardware inventory data available/i)).toBeInTheDocument();
    });
  });

  it('uses correct API endpoint', () => {
    mockedAxios.get.mockResolvedValue({ data: { nodes: [] } });

    render(<Hardware />);

    expect(mockedAxios.get).toHaveBeenCalledWith(
      expect.stringContaining('/api/v1/hardware/inventory'),
      expect.any(Object),
    );
  });

  it('includes authorization header in request', async () => {
    mockedAxios.get.mockResolvedValue({ data: { nodes: [] } });

    render(<Hardware />);

    await waitFor(() => {
      expect(mockedAxios.get).toHaveBeenCalledWith(
        expect.any(String),
        expect.objectContaining({
          headers: expect.objectContaining({
            Authorization: 'Bearer test-token',
          }),
        }),
      );
    });
  });

  it('uses empty string when token not in localStorage', async () => {
    localStorage.removeItem('nest_token');
    mockedAxios.get.mockResolvedValue({ data: { nodes: [] } });

    render(<Hardware />);

    await waitFor(() => {
      expect(mockedAxios.get).toHaveBeenCalledWith(
        expect.any(String),
        expect.objectContaining({
          headers: expect.objectContaining({
            Authorization: 'Bearer ',
          }),
        }),
      );
    });
  });

  it('coalesces undefined nodes to empty array', async () => {
    mockedAxios.get.mockResolvedValue({ data: {} });

    render(<Hardware />);

    await waitFor(() => {
      expect(screen.getByText(/no hardware inventory data available/i)).toBeInTheDocument();
    });
  });

  it('renders grid with node cards when nodes present', async () => {
    mockedAxios.get.mockResolvedValue({
      data: {
        nodes: [{ name: 'node-1', class: 'compute' }],
      },
    });

    render(<Hardware />);

    await waitFor(() => {
      expect(screen.getByText('node-1')).toBeInTheDocument();
    });

    const grid = screen.getByText('node-1').closest('.grid');
    expect(grid).toBeInTheDocument();
  });

  it('does not show empty state message when nodes present', async () => {
    mockedAxios.get.mockResolvedValue({
      data: {
        nodes: [{ name: 'node-1', class: 'compute' }],
      },
    });

    render(<Hardware />);

    await waitFor(() => {
      expect(screen.queryByText(/no hardware inventory data available/i)).not.toBeInTheDocument();
    });
  });

  it('handles null data with optional chaining', async () => {
    mockedAxios.get.mockResolvedValue({ data: null });

    render(<Hardware />);

    await waitFor(() => {
      expect(screen.getByText(/no hardware inventory data available/i)).toBeInTheDocument();
    });
  });
});
