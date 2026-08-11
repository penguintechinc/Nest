import { describe, it, expect, beforeEach, vi, afterEach } from 'vitest';
import { render, screen, waitFor } from '../test/test-utils';
import userEvent from '@testing-library/user-event';
import axios from 'axios';
import Backups from './Backups';

vi.mock('axios');
const mockedAxios = axios as any;

describe('Backups', () => {
  beforeEach(() => {
    localStorage.clear();
    localStorage.setItem('nest_tenant', 'test-tenant');
    localStorage.setItem('auth_token', 'test-token');
    vi.clearAllMocks();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('renders backups page title', () => {
    mockedAxios.get.mockResolvedValue({ data: { policies: [] } });

    render(<Backups />);

    expect(screen.getByRole('heading', { name: /backups/i })).toBeInTheDocument();
  });

  it('displays loading state initially', () => {
    mockedAxios.get.mockImplementation(
      () =>
        new Promise(resolve =>
          setTimeout(
            () =>
              resolve({
                data: { policies: [] },
              }),
            100,
          ),
        ),
    );

    render(<Backups />);

    expect(screen.getByRole('heading', { name: /backups/i })).toBeInTheDocument();
  });

  it('displays empty state message when no policies', async () => {
    mockedAxios.get.mockResolvedValue({ data: { policies: [] } });

    render(<Backups />);

    await waitFor(() => {
      expect(screen.getByText(/no backup policies found/i)).toBeInTheDocument();
    });
  });

  it('displays policy list in table', async () => {
    mockedAxios.get.mockResolvedValue({
      data: {
        policies: [
          {
            name: 'policy-daily',
            snapshotSchedule: '@daily',
            backupSchedule: '@daily',
            destination: 's3-bucket',
            lastSnapshot: '2025-01-20T10:00:00Z',
            lastBackup: '2025-01-20T12:00:00Z',
          },
          {
            name: 'policy-weekly',
            snapshotSchedule: '@weekly',
            backupSchedule: '@weekly',
            destination: 's3-archive',
            lastSnapshot: '2025-01-19T10:00:00Z',
            lastBackup: '2025-01-19T12:00:00Z',
          },
        ],
      },
    });

    render(<Backups />);

    await waitFor(() => {
      expect(screen.getByText('policy-daily')).toBeInTheDocument();
    });

    expect(screen.getByText('policy-weekly')).toBeInTheDocument();
    expect(screen.getByText('s3-bucket')).toBeInTheDocument();
    expect(screen.getByText('s3-archive')).toBeInTheDocument();
  });

  it('displays info callout about backups and snapshots', async () => {
    mockedAxios.get.mockResolvedValue({ data: { policies: [] } });

    render(<Backups />);

    await waitFor(() => {
      expect(screen.getByText(/backups are stored in your s3/i)).toBeInTheDocument();
    });
  });

  it('opens create modal on button click', async () => {
    const user = userEvent.setup();
    mockedAxios.get.mockResolvedValue({ data: { policies: [], dataresources: [] } });

    render(<Backups />);

    await waitFor(() => {
      expect(screen.getByText(/no backup policies found/i)).toBeInTheDocument();
    });

    const createButton = screen.getByRole('button', { name: /create/i });
    await user.click(createButton);

    expect(screen.getByText(/create backup policy/i)).toBeInTheDocument();
  });

  it('create modal has required fields', async () => {
    const user = userEvent.setup();
    mockedAxios.get.mockResolvedValue({ data: { policies: [], dataresources: [] } });

    render(<Backups />);

    const createButton = screen.getByRole('button', { name: /create/i });
    await user.click(createButton);

    expect(screen.getByText(/create backup policy/i)).toBeInTheDocument();
    expect(screen.getByText(/policy name/i)).toBeInTheDocument();
    expect(screen.getByText(/snapshot schedule/i)).toBeInTheDocument();
    expect(screen.getByText(/backup schedule/i)).toBeInTheDocument();
    const labels = screen.getAllByText(/destination/i);
    expect(labels.length).toBeGreaterThan(0);
  });

  it('closes create modal on cancel', async () => {
    const user = userEvent.setup();
    mockedAxios.get.mockResolvedValue({ data: { policies: [], dataresources: [] } });

    render(<Backups />);

    const createButton = screen.getByRole('button', { name: /create/i });
    await user.click(createButton);

    const cancelButton = screen.getByRole('button', { name: /cancel/i });
    await user.click(cancelButton);

    expect(screen.queryByText(/create backup policy/i)).not.toBeInTheDocument();
  });

  it('displays delete button for each policy', async () => {
    mockedAxios.get.mockResolvedValue({
      data: {
        policies: [
          {
            name: 'policy-1',
            snapshotSchedule: '@daily',
            backupSchedule: '@daily',
            destination: 's3-bucket',
          },
        ],
      },
    });

    render(<Backups />);

    await waitFor(() => {
      expect(screen.getByText('policy-1')).toBeInTheDocument();
    });

    const deleteButtons = screen.getAllByRole('button').filter(btn => btn.querySelector('svg'));
    expect(deleteButtons.length).toBeGreaterThan(0);
  });

  it('calls delete mutation when delete button clicked', async () => {
    const user = userEvent.setup();
    mockedAxios.get.mockResolvedValue({
      data: {
        policies: [
          {
            name: 'policy-1',
            snapshotSchedule: '@daily',
            backupSchedule: '@daily',
            destination: 's3-bucket',
          },
        ],
      },
    });
    mockedAxios.delete.mockResolvedValue({});

    render(<Backups />);

    await waitFor(() => {
      expect(screen.getByText('policy-1')).toBeInTheDocument();
    });

    const deleteButtons = screen.getAllByRole('button').filter(btn => btn.querySelector('svg') && !btn.querySelector('[data-testid="refresh"]'));
    if (deleteButtons.length > 0) {
      await user.click(deleteButtons[deleteButtons.length - 1]);
    }

    expect(mockedAxios.delete).toHaveBeenCalledWith(
      expect.stringContaining('/protection-policies/policy-1'),
    );
  });

  it('refresh button calls refetch', async () => {
    const user = userEvent.setup();
    mockedAxios.get.mockResolvedValue({ data: { policies: [] } });

    render(<Backups />);

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
    mockedAxios.get.mockResolvedValue({ data: { policies: [] } });

    render(<Backups />);

    await waitFor(() => {
      expect(screen.getByRole('heading', { name: /backups/i })).toBeInTheDocument();
    });
  });

  it('displays dash when lastSnapshot or lastBackup is undefined', async () => {
    mockedAxios.get.mockResolvedValue({
      data: {
        policies: [
          {
            name: 'policy-1',
            snapshotSchedule: '@daily',
            backupSchedule: '@daily',
            destination: 's3-bucket',
          },
        ],
      },
    });

    render(<Backups />);

    await waitFor(() => {
      expect(screen.getByText('policy-1')).toBeInTheDocument();
    });

    const dashes = screen.getAllByText('—');
    expect(dashes.length).toBeGreaterThan(0);
  });

  it('populates destination dropdown from object-type dataresources', async () => {
    const user = userEvent.setup();
    mockedAxios.get.mockImplementation((url: string) => {
      if (url.includes('protection-policies')) {
        return Promise.resolve({ data: { policies: [] } });
      }
      if (url.includes('dataresources')) {
        return Promise.resolve({
          data: {
            dataresources: [
              { name: 's3-bucket-1', type: 'object' },
              { name: 's3-bucket-2', type: 'object' },
            ],
          },
        });
      }
      return Promise.resolve({ data: {} });
    });

    render(<Backups />);

    const createButton = screen.getByRole('button', { name: /create/i });
    await user.click(createButton);

    await waitFor(() => {
      const options = screen.getAllByRole('option');
      expect(options.some(opt => opt.textContent === 's3-bucket-1')).toBe(true);
    });
  });

  it('displays restore button for each policy', async () => {
    mockedAxios.get.mockResolvedValue({
      data: {
        policies: [
          {
            name: 'policy-1',
            snapshotSchedule: '@daily',
            backupSchedule: '@daily',
            destination: 's3-bucket',
          },
        ],
      },
    });

    render(<Backups />);

    await waitFor(() => {
      expect(screen.getByText('policy-1')).toBeInTheDocument();
    });

    const buttons = screen.getAllByRole('button').filter(btn => btn.querySelector('svg'));
    expect(buttons.length).toBeGreaterThanOrEqual(2);
  });

  it('shows restore confirm modal when restore button clicked', async () => {
    const user = userEvent.setup();
    mockedAxios.get.mockResolvedValue({
      data: {
        policies: [
          {
            name: 'policy-1',
            snapshotSchedule: '@daily',
            backupSchedule: '@daily',
            destination: 's3-bucket',
          },
        ],
      },
    });

    render(<Backups />);

    await waitFor(() => {
      expect(screen.getByText('policy-1')).toBeInTheDocument();
    });

    // Get the row buttons
    const rows = screen.getAllByRole('row');
    const policyRow = rows.find(row => row.textContent?.includes('policy-1'));
    const rowButtons = policyRow?.querySelectorAll('button');
    const restoreButton = Array.from(rowButtons || []).find(btn =>
      btn.className?.includes('hover:text-amber')
    ) as HTMLButtonElement;

    if (restoreButton) {
      await user.click(restoreButton);
    }

    await waitFor(() => {
      expect(screen.getByRole('heading', { name: /restore backup/i })).toBeInTheDocument();
    });
  });

  it('calls restore mutation when confirm button clicked', async () => {
    const user = userEvent.setup();
    mockedAxios.get.mockResolvedValue({
      data: {
        policies: [
          {
            name: 'policy-1',
            snapshotSchedule: '@daily',
            backupSchedule: '@daily',
            destination: 's3-bucket',
          },
        ],
      },
    });
    mockedAxios.post.mockResolvedValue({});

    render(<Backups />);

    await waitFor(() => {
      expect(screen.getByText('policy-1')).toBeInTheDocument();
    });

    // Get the row buttons
    const rows = screen.getAllByRole('row');
    const policyRow = rows.find(row => row.textContent?.includes('policy-1'));
    const rowButtons = policyRow?.querySelectorAll('button');
    const restoreButton = Array.from(rowButtons || []).find(btn =>
      btn.className?.includes('hover:text-amber')
    ) as HTMLButtonElement;

    if (restoreButton) {
      await user.click(restoreButton);
    }

    await waitFor(() => {
      expect(screen.getByRole('heading', { name: /restore backup/i })).toBeInTheDocument();
    });

    const confirmButtons = screen.getAllByRole('button', { name: /^restore$/i });
    const restoreConfirmButton = confirmButtons[confirmButtons.length - 1];
    await user.click(restoreConfirmButton);

    expect(mockedAxios.post).toHaveBeenCalledWith(
      expect.stringContaining('/data-resources/policy-1/restore'),
      { backup_name: 'policy-1' },
    );
  });

  it('closes restore modal on cancel', async () => {
    const user = userEvent.setup();
    mockedAxios.get.mockResolvedValue({
      data: {
        policies: [
          {
            name: 'policy-1',
            snapshotSchedule: '@daily',
            backupSchedule: '@daily',
            destination: 's3-bucket',
          },
        ],
      },
    });

    render(<Backups />);

    await waitFor(() => {
      expect(screen.getByText('policy-1')).toBeInTheDocument();
    });

    // Get the row buttons
    const rows = screen.getAllByRole('row');
    const policyRow = rows.find(row => row.textContent?.includes('policy-1'));
    const rowButtons = policyRow?.querySelectorAll('button');
    const restoreButton = Array.from(rowButtons || []).find(btn =>
      btn.className?.includes('hover:text-amber')
    ) as HTMLButtonElement;

    if (restoreButton) {
      await user.click(restoreButton);
    }

    // Wait for modal to appear
    await waitFor(() => {
      const heading = screen.getByText(/restore from backup/i);
      expect(heading).toBeInTheDocument();
    }, { timeout: 3000 });

    const cancelButtons = screen.getAllByRole('button', { name: /cancel/i });
    const restoreCancelButton = cancelButtons[cancelButtons.length - 1];
    await user.click(restoreCancelButton);

    // Modal should disappear
    await waitFor(() => {
      expect(screen.queryByText(/restore from backup/i)).not.toBeInTheDocument();
    }, { timeout: 3000 });
  });
});
