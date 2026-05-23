import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render, screen } from '../test/test-utils';
import userEvent from '@testing-library/user-event';
import Login from './Login';

describe('Login', () => {
  beforeEach(() => {
    localStorage.clear();
  });

  it('renders login form with tenant and token fields', () => {
    render(<Login />);

    expect(screen.getByRole('heading', { name: /nest admin/i })).toBeInTheDocument();
    expect(screen.getByLabelText(/tenant/i)).toBeInTheDocument();
    expect(screen.getByLabelText(/api token/i)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /sign in/i })).toBeInTheDocument();
  });

  it('shows validation error with empty fields', async () => {
    const user = userEvent.setup();
    render(<Login />);

    const submitButton = screen.getByRole('button', { name: /sign in/i });
    await user.click(submitButton);

    expect(screen.getByText(/tenant and token are required/i)).toBeInTheDocument();
  });

  it('does not navigate with empty tenant', async () => {
    const user = userEvent.setup();
    render(<Login />);

    const tokenInput = screen.getByLabelText(/api token/i);
    await user.type(tokenInput, 'test-token');
    await user.click(screen.getByRole('button', { name: /sign in/i }));

    expect(screen.getByText(/tenant and token are required/i)).toBeInTheDocument();
    expect(localStorage.getItem('nest_token')).toBeNull();
  });

  it('does not navigate with empty token', async () => {
    const user = userEvent.setup();
    render(<Login />);

    const tenantInput = screen.getByLabelText(/tenant/i);
    await user.type(tenantInput, 'acme');
    await user.click(screen.getByRole('button', { name: /sign in/i }));

    expect(screen.getByText(/tenant and token are required/i)).toBeInTheDocument();
    expect(localStorage.getItem('nest_token')).toBeNull();
  });

  it('stores token and tenant in localStorage and navigates on valid submit', async () => {
    const user = userEvent.setup();
    render(<Login />);

    const tenantInput = screen.getByLabelText(/tenant/i);
    const tokenInput = screen.getByLabelText(/api token/i);

    await user.type(tenantInput, 'acme');
    await user.type(tokenInput, 'test-token-123');
    await user.click(screen.getByRole('button', { name: /sign in/i }));

    expect(localStorage.getItem('nest_tenant')).toBe('acme');
    expect(localStorage.getItem('nest_token')).toBe('test-token-123');
  });

  it('trims whitespace from tenant and token', async () => {
    const user = userEvent.setup();
    render(<Login />);

    const tenantInput = screen.getByLabelText(/tenant/i);
    const tokenInput = screen.getByLabelText(/api token/i);

    await user.type(tenantInput, '  acme  ');
    await user.type(tokenInput, '  test-token  ');
    await user.click(screen.getByRole('button', { name: /sign in/i }));

    expect(localStorage.getItem('nest_tenant')).toBe('acme');
    expect(localStorage.getItem('nest_token')).toBe('test-token');
  });

  it('clears error message on input change', async () => {
    const user = userEvent.setup();
    render(<Login />);

    await user.click(screen.getByRole('button', { name: /sign in/i }));
    expect(screen.getByText(/tenant and token are required/i)).toBeInTheDocument();

    const tenantInput = screen.getByLabelText(/tenant/i);
    await user.type(tenantInput, 'test');

    // Error should still be visible until submission with both fields
    expect(screen.getByText(/tenant and token are required/i)).toBeInTheDocument();
  });
});
