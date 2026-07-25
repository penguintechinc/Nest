import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render, screen } from '../../test/test-utils';
import userEvent from '@testing-library/user-event';

/**
 * Covers this app's onSuccess handler — what it persists after the shared
 * LoginPageBuilder reports a successful login. The builder itself is stubbed so
 * these assertions cover our code, not the library's form.
 */
const mockNavigate = vi.fn();
vi.mock('react-router-dom', async () => {
  const actual = await vi.importActual<typeof import('react-router-dom')>('react-router-dom');
  return { ...actual, useNavigate: () => mockNavigate };
});

let loginResponse: Record<string, unknown>;
vi.mock('@penguintechinc/react-libs', () => ({
  LoginPageBuilder: ({ onSuccess }: { onSuccess: (r: unknown) => void }) => (
    <button type="button" onClick={() => onSuccess(loginResponse)}>
      succeed
    </button>
  ),
}));

const LoginForm = (await import('./LoginForm')).default;

describe('LoginForm onSuccess', () => {
  beforeEach(() => {
    localStorage.clear();
    mockNavigate.mockClear();
  });

  it('persists the token and the tenant returned by the auth service', async () => {
    const user = userEvent.setup();
    loginResponse = {
      success: true,
      token: 'jwt-abc',
      tenant: 'acme',
      user: { id: 'u1', email: 'user@example.com', name: 'User' },
    };

    render(<LoginForm />);
    await user.click(screen.getByRole('button', { name: /succeed/i }));

    expect(localStorage.getItem('auth_token')).toBe('jwt-abc');
    expect(localStorage.getItem('nest_tenant')).toBe('acme');
    expect(mockNavigate).toHaveBeenCalledWith('/dashboard');
  });

  it('falls back to the default tenant when the service omits one', async () => {
    const user = userEvent.setup();
    loginResponse = { success: true, token: 'jwt-abc' };

    render(<LoginForm />);
    await user.click(screen.getByRole('button', { name: /succeed/i }));

    expect(localStorage.getItem('nest_tenant')).toBe('nest');
  });

  it('ignores a tenant supplied only by the client', async () => {
    const user = userEvent.setup();
    // The tenant must come from the auth service; a typed value never overrides it.
    localStorage.setItem('nest_tenant', 'stale');
    loginResponse = { success: true, token: 'jwt-abc', tenant: 'authoritative' };

    render(<LoginForm />);
    await user.click(screen.getByRole('button', { name: /succeed/i }));

    expect(localStorage.getItem('nest_tenant')).toBe('authoritative');
  });
});
