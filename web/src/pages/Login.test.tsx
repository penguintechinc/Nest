import { describe, it, expect, beforeEach } from 'vitest';
import { render, screen } from '../test/test-utils';
import Login from './Login';

/**
 * Covers this app's configuration of the shared LoginPageBuilder: that the
 * multi-tenant field is enabled and pre-filled. The form's own behaviour is
 * owned and tested by @penguintechinc/react-libs, not re-tested here.
 */
describe('Login', () => {
  beforeEach(() => {
    localStorage.clear();
  });

  it('renders tenant, email, password, and submit', () => {
    render(<Login />);

    expect(screen.getByLabelText(/tenant/i)).toBeInTheDocument();
    expect(screen.getByLabelText(/email/i)).toBeInTheDocument();
    expect(screen.getByLabelText(/password/i)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /sign in/i })).toBeInTheDocument();
  });

  it('defaults the tenant so users need not know it', () => {
    render(<Login />);

    expect(screen.getByLabelText(/tenant/i)).toHaveValue('nest');
  });

  it('does not ask users for an API key', () => {
    render(<Login />);

    expect(screen.queryByLabelText(/api key/i)).not.toBeInTheDocument();
    expect(screen.queryByLabelText(/api token/i)).not.toBeInTheDocument();
  });
});
