/**
 * Resource management service
 * Handles CRUD operations for cloud resources
 */

import api from './api';
import { Resource, ResourceList, CreateResourceInput } from '../types/resource';

class ResourceService {
  /**
   * Get all resources with pagination
   */
  async getResources(page = 1, per_page = 20): Promise<ResourceList> {
    try {
      const response = await api.get<ResourceList>('/resources', {
        params: { page, per_page },
      });
      return response.data;
    } catch (_error) {
      throw new Error('Failed to fetch resources');
    }
  }

  /**
   * Get resources filtered by lifecycle mode
   */
  async getResourcesByLifecycle(
    lifecycle_mode: 'full' | 'partial' | 'monitor_only',
    page = 1,
    per_page = 20
  ): Promise<ResourceList> {
    try {
      const response = await api.get<ResourceList>('/resources', {
        params: { lifecycle_mode, page, per_page },
      });
      return response.data;
    } catch (_error) {
      throw new Error('Failed to fetch resources by lifecycle');
    }
  }

  /**
   * Get single resource by ID
   */
  async getResource(id: string): Promise<Resource> {
    try {
      const response = await api.get<Resource>(`/resources/${id}`);
      return response.data;
    } catch (_error) {
      throw new Error('Failed to fetch resource');
    }
  }

  /**
   * Create new resource
   */
  async createResource(data: CreateResourceInput): Promise<Resource> {
    try {
      const response = await api.post<Resource>('/resources', data);
      return response.data;
    } catch (_error) {
      throw new Error('Failed to create resource');
    }
  }

  /**
   * Update existing resource
   */
  async updateResource(id: string, data: Partial<CreateResourceInput>): Promise<Resource> {
    try {
      const response = await api.put<Resource>(`/resources/${id}`, data);
      return response.data;
    } catch (_error) {
      throw new Error('Failed to update resource');
    }
  }

  /**
   * Delete resource
   */
  async deleteResource(id: string): Promise<void> {
    try {
      await api.delete(`/resources/${id}`);
    } catch (_error) {
      throw new Error('Failed to delete resource');
    }
  }

  /**
   * Get resource statistics
   */
  async getResourceStats(): Promise<{
    total: number;
    by_lifecycle: Record<string, number>;
    by_provider: Record<string, number>;
    by_status: Record<string, number>;
  }> {
    try {
      const response = await api.get('/resources/stats');
      return response.data;
    } catch (_error) {
      throw new Error('Failed to fetch resource statistics');
    }
  }
}

export default new ResourceService();
