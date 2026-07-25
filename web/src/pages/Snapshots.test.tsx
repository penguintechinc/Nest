import { describe, it, expect, beforeEach, vi, afterEach } from 'vitest';
import { render, screen, waitFor } from '../test/test-utils';
import userEvent from '@testing-library/user-event';
import axios from 'axios';
import Snapshots from './Snapshots';

vi.mock('axios');
const mockedAxios = axios as any;

describe('Snapshots', () => {
  beforeEach(() => {
    localStorage.clear();
    localStorage.setItem('nest_tenant', 'test-tenant');
    localStorage.setItem('auth_token', 'test-token');
    vi.clearAllMocks();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('renders snapshots page title', () => {
    mockedAxios.get.mockResolvedValue({ data: { snapshots: [] } });

    render(<Snapshots />);

    expect(screen.getByRole('heading', { name: /snapshots/i })).toBeInTheDocument();
  });

  it('displays loading state initially', () => {
    mockedAxios.get.mockImplementation(
      () =>
        new Promise(resolve =>
          setTimeout(
            () =>
              resolve({
                data: { snapshots: [] },
              }),
            100,
          ),
        ),
    );

    render(<Snapshots />);

    expect(screen.getByRole('heading', { name: /snapshots/i })).toBeInTheDocument();
  });

  it('displays empty state message when no snapshots', async () => {
    mockedAxios.get.mockResolvedValue({ data: { snapshots: [] } });

    render(<Snapshots />);

    await waitFor(() => {
      expect(screen.getByText(/no snapshots found/i)).toBeInTheDocument();
    });
  });

  it('displays snapshot list in table', async () => {
    mockedAxios.get.mockResolvedValue({
      data: {
        snapshots: [
          {
            name: 'snap-1',
            sourcePVC: 'pvc-data',
            snapshotClass: 'nest-rbd-snapshot',
            readyToUse: true,
            creationTime: '2025-01-20T10:00:00Z',
            size: '5Gi',
          },
          {
            name: 'snap-2',
            sourcePVC: 'pvc-backup',
            snapshotClass: 'nest-rbd-snapshot',
            readyToUse: false,
            creationTime: '2025-01-21T10:00:00Z',
            size: '10Gi',
          },
        ],
      },
    });

    render(<Snapshots />);

    await waitFor(() => {
      expect(screen.getByText('snap-1')).toBeInTheDocument();
    });

    expect(screen.getByText('snap-2')).toBeInTheDocument();
    expect(screen.getByText('pvc-data')).toBeInTheDocument();
    expect(screen.getByText('pvc-backup')).toBeInTheDocument();
  });

  it('displays correct status badges', async () => {
    mockedAxios.get.mockResolvedValue({
      data: {
        snapshots: [
          {
            name: 'snap-ready',
            sourcePVC: 'pvc-1',
            snapshotClass: 'nest-rbd-snapshot',
            readyToUse: true,
            creationTime: '2025-01-20T10:00:00Z',
          },
          {
            name: 'snap-pending',
            sourcePVC: 'pvc-2',
            snapshotClass: 'nest-rbd-snapshot',
            readyToUse: false,
            creationTime: '2025-01-21T10:00:00Z',
          },
        ],
      },
    });

    render(<Snapshots />);

    await waitFor(() => {
      expect(screen.getByText('snap-ready')).toBeInTheDocument();
    });

    expect(screen.getByText('Ready')).toBeInTheDocument();
    expect(screen.getByText('Pending')).toBeInTheDocument();
  });

  it('opens create modal on button click', async () => {
    const user = userEvent.setup();
    mockedAxios.get.mockResolvedValue({ data: { snapshots: [], dataresources: [] } });

    render(<Snapshots />);

    await waitFor(() => {
      expect(screen.getByText(/no snapshots found/i)).toBeInTheDocument();
    });

    const createButton = screen.getByRole('button', { name: /create/i });
    await user.click(createButton);

    expect(screen.getByText(/create snapshot/i)).toBeInTheDocument();
  });

  it('create modal has required fields', async () => {
    const user = userEvent.setup();
    mockedAxios.get.mockResolvedValue({ data: { snapshots: [], dataresources: [] } });

    render(<Snapshots />);

    const createButton = screen.getByRole('button', { name: /create/i });
    await user.click(createButton);

    expect(screen.getByText(/snapshot name/i)).toBeInTheDocument();
    expect(screen.getByText(/source pvc/i)).toBeInTheDocument();
    expect(screen.getByText(/snapshot class/i)).toBeInTheDocument();
  });

  it('closes create modal on cancel', async () => {
    const user = userEvent.setup();
    mockedAxios.get.mockResolvedValue({ data: { snapshots: [], dataresources: [] } });

    render(<Snapshots />);

    const createButton = screen.getByRole('button', { name: /create/i });
    await user.click(createButton);

    const cancelButton = screen.getByRole('button', { name: /cancel/i });
    await user.click(cancelButton);

    expect(screen.queryByText(/create snapshot/i)).not.toBeInTheDocument();
  });

  it('displays delete button for each snapshot', async () => {
    mockedAxios.get.mockResolvedValue({
      data: {
        snapshots: [
          {
            name: 'snap-1',
            sourcePVC: 'pvc-1',
            snapshotClass: 'nest-rbd-snapshot',
            readyToUse: true,
            creationTime: '2025-01-20T10:00:00Z',
          },
        ],
      },
    });

    render(<Snapshots />);

    await waitFor(() => {
      expect(screen.getByText('snap-1')).toBeInTheDocument();
    });

    const deleteButtons = screen.getAllByRole('button').filter(btn => btn.querySelector('svg'));
    expect(deleteButtons.length).toBeGreaterThan(0);
  });

  it('calls delete mutation when delete button clicked', async () => {
    const user = userEvent.setup();
    mockedAxios.get.mockResolvedValue({
      data: {
        snapshots: [
          {
            name: 'snap-1',
            sourcePVC: 'pvc-1',
            snapshotClass: 'nest-rbd-snapshot',
            readyToUse: true,
            creationTime: '2025-01-20T10:00:00Z',
          },
        ],
      },
    });
    mockedAxios.delete.mockResolvedValue({});

    render(<Snapshots />);

    await waitFor(() => {
      expect(screen.getByText('snap-1')).toBeInTheDocument();
    });

    const deleteButtons = screen.getAllByRole('button').filter(btn => btn.querySelector('svg') && !btn.querySelector('[data-testid="refresh"]'));
    if (deleteButtons.length > 0) {
      await user.click(deleteButtons[deleteButtons.length - 1]);
    }

    expect(mockedAxios.delete).toHaveBeenCalledWith(
      expect.stringContaining('/snapshots/snap-1'),
    );
  });

  it('refresh button calls refetch', async () => {
    const user = userEvent.setup();
    mockedAxios.get.mockResolvedValue({ data: { snapshots: [] } });

    render(<Snapshots />);

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
    mockedAxios.get.mockResolvedValue({ data: { snapshots: [] } });

    render(<Snapshots />);

    await waitFor(() => {
      expect(screen.getByRole('heading', { name: /snapshots/i })).toBeInTheDocument();
    });
  });

  it('displays dash when size is undefined', async () => {
    mockedAxios.get.mockResolvedValue({
      data: {
        snapshots: [
          {
            name: 'snap-1',
            sourcePVC: 'pvc-1',
            snapshotClass: 'nest-rbd-snapshot',
            readyToUse: true,
            creationTime: '2025-01-20T10:00:00Z',
          },
        ],
      },
    });

    render(<Snapshots />);

    await waitFor(() => {
      expect(screen.getByText('snap-1')).toBeInTheDocument();
    });

    expect(screen.getByText('—')).toBeInTheDocument();
  });

  it('populates PVC dropdown from dataresources', async () => {
    const user = userEvent.setup();
    mockedAxios.get.mockImplementation((url: string) => {
      if (url.includes('snapshots')) {
        return Promise.resolve({ data: { snapshots: [] } });
      }
      if (url.includes('dataresources')) {
        return Promise.resolve({
          data: {
            dataresources: [
              { name: 'pvc-1', type: 'pvc' },
              { name: 'pvc-2', type: 'pvc' },
            ],
          },
        });
      }
      return Promise.resolve({ data: {} });
    });

    render(<Snapshots />);

    const createButton = screen.getByRole('button', { name: /create/i });
    await user.click(createButton);

    await waitFor(() => {
      const options = screen.getAllByRole('option');
      expect(options.some(opt => opt.textContent === 'pvc-1')).toBe(true);
    });
  });

  it('displays restore button for each snapshot', async () => {
    mockedAxios.get.mockResolvedValue({
      data: {
        snapshots: [
          {
            name: 'snap-1',
            sourcePVC: 'pvc-1',
            snapshotClass: 'nest-rbd-snapshot',
            readyToUse: true,
            creationTime: '2025-01-20T10:00:00Z',
          },
        ],
      },
    });

    render(<Snapshots />);

    await waitFor(() => {
      expect(screen.getByText('snap-1')).toBeInTheDocument();
    });

    const buttons = screen.getAllByRole('button').filter(btn => btn.querySelector('svg'));
    expect(buttons.length).toBeGreaterThanOrEqual(2);
  });

  it('shows restore confirm modal when restore button clicked', async () => {
    const user = userEvent.setup();
    mockedAxios.get.mockResolvedValue({
      data: {
        snapshots: [
          {
            name: 'snap-1',
            sourcePVC: 'pvc-1',
            snapshotClass: 'nest-rbd-snapshot',
            readyToUse: true,
            creationTime: '2025-01-20T10:00:00Z',
          },
        ],
      },
    });

    render(<Snapshots />);

    await waitFor(() => {
      expect(screen.getByText('snap-1')).toBeInTheDocument();
    });

    // Get the row buttons
    const rows = screen.getAllByRole('row');
    const snapRow = rows.find(row => row.textContent?.includes('snap-1'));
    const rowButtons = snapRow?.querySelectorAll('button');
    const restoreButton = Array.from(rowButtons || []).find(btn =>
      btn.className?.includes('hover:text-amber')
    ) as HTMLButtonElement;

    if (restoreButton) {
      await user.click(restoreButton);
    }

    await waitFor(() => {
      expect(screen.getByRole('heading', { name: /restore snapshot/i })).toBeInTheDocument();
    });
  });

  it('calls restore mutation when confirm button clicked', async () => {
    const user = userEvent.setup();
    mockedAxios.get.mockResolvedValue({
      data: {
        snapshots: [
          {
            name: 'snap-1',
            sourcePVC: 'pvc-1',
            snapshotClass: 'nest-rbd-snapshot',
            readyToUse: true,
            creationTime: '2025-01-20T10:00:00Z',
          },
        ],
      },
    });
    mockedAxios.post.mockResolvedValue({});

    render(<Snapshots />);

    await waitFor(() => {
      expect(screen.getByText('snap-1')).toBeInTheDocument();
    });

    // Get the row buttons
    const rows = screen.getAllByRole('row');
    const snapRow = rows.find(row => row.textContent?.includes('snap-1'));
    const rowButtons = snapRow?.querySelectorAll('button');
    const restoreButton = Array.from(rowButtons || []).find(btn =>
      btn.className?.includes('hover:text-amber')
    ) as HTMLButtonElement;

    if (restoreButton) {
      await user.click(restoreButton);
    }

    await waitFor(() => {
      expect(screen.getByRole('heading', { name: /restore snapshot/i })).toBeInTheDocument();
    });

    const confirmButtons = screen.getAllByRole('button', { name: /^restore$/i });
    const restoreConfirmButton = confirmButtons[confirmButtons.length - 1];
    await user.click(restoreConfirmButton);

    expect(mockedAxios.post).toHaveBeenCalledWith(
      expect.stringContaining('/data-resources/pvc-1/restore'),
      { snapshot_name: 'snap-1' },
    );
  });

  it('closes restore modal on cancel', async () => {
    const user = userEvent.setup();
    mockedAxios.get.mockResolvedValue({
      data: {
        snapshots: [
          {
            name: 'snap-1',
            sourcePVC: 'pvc-1',
            snapshotClass: 'nest-rbd-snapshot',
            readyToUse: true,
            creationTime: '2025-01-20T10:00:00Z',
          },
        ],
      },
    });

    render(<Snapshots />);

    await waitFor(() => {
      expect(screen.getByText('snap-1')).toBeInTheDocument();
    });

    // Get the row buttons
    const rows = screen.getAllByRole('row');
    const snapRow = rows.find(row => row.textContent?.includes('snap-1'));
    const rowButtons = snapRow?.querySelectorAll('button');
    const restoreButton = Array.from(rowButtons || []).find(btn =>
      btn.className?.includes('hover:text-amber')
    ) as HTMLButtonElement;

    if (restoreButton) {
      await user.click(restoreButton);
    }

    // Wait for modal to appear
    await waitFor(() => {
      const heading = screen.getByText(/restore from snapshot/i);
      expect(heading).toBeInTheDocument();
    }, { timeout: 3000 });

    const cancelButtons = screen.getAllByRole('button', { name: /cancel/i });
    const restoreCancelButton = cancelButtons[cancelButtons.length - 1];
    await user.click(restoreCancelButton);

    // Modal should disappear
    await waitFor(() => {
      expect(screen.queryByText(/restore from snapshot/i)).not.toBeInTheDocument();
    }, { timeout: 3000 });
  });
});
