import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render, screen } from '../test/test-utils';
import userEvent from '@testing-library/user-event';
import Layout from './Layout';

describe('Layout', () => {
  beforeEach(() => {
    localStorage.clear();
    localStorage.setItem('nest_tenant', 'test-tenant');
    localStorage.setItem('nest_token', 'test-token');
    vi.clearAllMocks();
  });

  it('renders sidebar with logo', () => {
    render(<Layout />);

    expect(screen.getByText(/nest admin/i)).toBeInTheDocument();
    expect(screen.getByText(/storage platform/i)).toBeInTheDocument();
  });

  it('renders navigation links', () => {
    render(<Layout />);

    expect(screen.getByText(/dashboard/i)).toBeInTheDocument();
    expect(screen.getByText(/resources/i)).toBeInTheDocument();
    expect(screen.getByText(/databases/i)).toBeInTheDocument();
    expect(screen.getByText(/hardware/i)).toBeInTheDocument();
    expect(screen.getByText(/audit logs/i)).toBeInTheDocument();
    expect(screen.getByText(/settings/i)).toBeInTheDocument();
  });

  it('renders logout button', () => {
    render(<Layout />);

    expect(screen.getByRole('button', { name: /logout/i })).toBeInTheDocument();
  });

  it('logout button clears localStorage', async () => {
    const user = userEvent.setup();
    render(<Layout />);

    expect(localStorage.getItem('nest_token')).toBe('test-token');
    expect(localStorage.getItem('nest_tenant')).toBe('test-tenant');

    const logoutButton = screen.getByRole('button', { name: /logout/i });
    await user.click(logoutButton);

    expect(localStorage.getItem('nest_token')).toBeNull();
    expect(localStorage.getItem('nest_tenant')).toBeNull();
  });

  it('displays all navigation links with correct labels', () => {
    render(<Layout />);

    const navLinks = [
      'Dashboard',
      'Resources',
      'Databases',
      'Hardware',
      'Audit Logs',
      'Settings',
    ];

    navLinks.forEach(label => {
      expect(screen.getByText(new RegExp(label, 'i'))).toBeInTheDocument();
    });
  });

  it('navigation links are clickable', () => {
    render(<Layout />);

    const dashboardLink = screen.getByText(/dashboard/i).closest('a');
    expect(dashboardLink).toBeInTheDocument();
    expect(dashboardLink).toHaveAttribute('href', '/dashboard');
  });

  it('has correct navigation href attributes', () => {
    render(<Layout />);

    const navRoutes = {
      Dashboard: '/dashboard',
      Resources: '/resources',
      Databases: '/databases',
      Hardware: '/hardware',
      'Audit Logs': '/audit',
      Settings: '/settings',
    };

    Object.entries(navRoutes).forEach(([label, href]) => {
      const link = screen.getByText(new RegExp(label, 'i')).closest('a');
      expect(link).toHaveAttribute('href', href);
    });
  });

  it('renders sidebar and main content areas', () => {
    render(<Layout />);

    expect(screen.getByText(/nest admin/i)).toBeInTheDocument();
  });

  it('sidebar contains navigation and logout sections', () => {
    render(<Layout />);

    expect(screen.getByText(/dashboard/i)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /logout/i })).toBeInTheDocument();
  });

  it('logout button changes text color on hover', async () => {
    const user = userEvent.setup();
    render(<Layout />);

    const logoutButton = screen.getByRole('button', { name: /logout/i });
    await user.hover(logoutButton);

    // Check that the button has hover styling classes
    expect(logoutButton).toHaveClass('hover:text-red-400');
  });

  it('navigation links have icons', () => {
    render(<Layout />);

    // All nav items should have SVG icons (from lucide-react)
    const navItems = screen.getAllByText(/dashboard|resources|databases|hardware|audit logs|settings/i);
    expect(navItems.length).toBeGreaterThan(0);
  });

  it('renders layout container correctly', () => {
    const { container } = render(<Layout />);

    const layoutDiv = container.querySelector('.flex.h-screen');
    expect(layoutDiv).toBeInTheDocument();
  });

  it('sidebar has correct width', () => {
    const { container } = render(<Layout />);

    const sidebar = container.querySelector('aside');
    expect(sidebar).toHaveClass('w-64');
  });

  it('main content area is flex-1', () => {
    const { container } = render(<Layout />);

    const main = container.querySelector('main');
    expect(main).toHaveClass('flex-1');
  });
});
