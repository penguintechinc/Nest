import { describe, it, expect, beforeEach } from 'vitest';
import { render, screen } from '../test/test-utils';
import Settings from './Settings';

describe('Settings', () => {
  beforeEach(() => {
    localStorage.clear();
    localStorage.setItem('nest_tenant', 'test-tenant');
  });

  it('renders settings page title', () => {
    render(<Settings />);

    expect(screen.getByRole('heading', { name: /settings/i })).toBeInTheDocument();
  });

  it('displays connection section heading', () => {
    render(<Settings />);

    expect(screen.getByText(/connection/i)).toBeInTheDocument();
  });

  it('displays tenant information from localStorage', () => {
    render(<Settings />);

    expect(screen.getAllByText(/tenant/i).length).toBeGreaterThan(0);
    expect(screen.getByText('test-tenant')).toBeInTheDocument();
  });

  it('displays API endpoint information', () => {
    render(<Settings />);

    expect(screen.getByText(/api endpoint/i)).toBeInTheDocument();
  });

  it('displays current location as API endpoint', () => {
    render(<Settings />);

    const endpoint = window.location.origin;
    expect(screen.getByText(endpoint)).toBeInTheDocument();
  });

  it('renders connection info in card container', () => {
    render(<Settings />);

    const headings = screen.getAllByText(/tenant|api endpoint/i);
    expect(headings.length).toBeGreaterThan(0);
  });

  it('shows empty string when no tenant in localStorage', () => {
    localStorage.clear();
    render(<Settings />);

    const tenantDisplay = screen.queryByText(/test-tenant/);
    expect(tenantDisplay).not.toBeInTheDocument();
  });

  it('displays tenant and endpoint in monospace font (font-mono)', () => {
    render(<Settings />);

    const tenantElement = screen.getByText('test-tenant');
    expect(tenantElement).toHaveClass('font-mono');

    const endpoint = window.location.origin;
    const endpointElement = screen.getByText(endpoint);
    expect(endpointElement).toHaveClass('font-mono');
  });
});
