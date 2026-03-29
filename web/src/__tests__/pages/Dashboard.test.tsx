/**
 * Unit tests for Dashboard page component
 */

import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import React from 'react';

// Mock child components to isolate Dashboard tests
vi.mock('../../components/stats/ResourceStats', () => ({
  default: () => <div data-testid="resource-stats">ResourceStats</div>,
}));

vi.mock('../../components/resources/ResourceList', () => ({
  default: ({ compact }: { compact?: boolean }) => (
    <div data-testid="resource-list" data-compact={compact}>
      ResourceList
    </div>
  ),
}));

import Dashboard from '../../pages/Dashboard';

describe('Dashboard', () => {
  it('should render the Dashboard heading', () => {
    render(<Dashboard />);
    expect(screen.getByText('Dashboard')).toBeDefined();
  });

  it('should render the welcome subtitle', () => {
    render(<Dashboard />);
    expect(
      screen.getByText('Welcome to NEST - Cloud Resource Management'),
    ).toBeDefined();
  });

  it('should render the ResourceStats component', () => {
    render(<Dashboard />);
    expect(screen.getByTestId('resource-stats')).toBeDefined();
  });

  it('should render the ResourceList component', () => {
    render(<Dashboard />);
    expect(screen.getByTestId('resource-list')).toBeDefined();
  });

  it('should pass compact=true to ResourceList', () => {
    render(<Dashboard />);
    const resourceList = screen.getByTestId('resource-list');
    expect(resourceList.getAttribute('data-compact')).toBe('true');
  });

  it('should render the Resources section heading', () => {
    render(<Dashboard />);
    expect(screen.getByText('Resources')).toBeDefined();
  });

  it('should render Quick Actions section', () => {
    render(<Dashboard />);
    expect(screen.getByText('Quick Actions')).toBeDefined();
    expect(screen.getByText('Create Resource')).toBeDefined();
    expect(screen.getByText('View All Resources')).toBeDefined();
    expect(screen.getByText('Manage Teams')).toBeDefined();
  });
});
