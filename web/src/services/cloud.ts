import api from './api';
import { CloudProvider, PaginatedResponse } from '../types/server';

class CloudService {
  async getProviders(page = 1, per_page = 20): Promise<PaginatedResponse<CloudProvider>> {
    const response = await api.get('/cloud/providers', { params: { page, per_page } });
    return response.data;
  }

  async getProvider(id: number): Promise<CloudProvider> {
    const response = await api.get(`/cloud/providers/${id}`);
    return response.data;
  }

  async createProvider(data: Partial<CloudProvider>): Promise<CloudProvider> {
    const response = await api.post('/cloud/providers', data);
    return response.data;
  }

  async deleteProvider(id: number): Promise<void> {
    await api.delete(`/cloud/providers/${id}`);
  }

  async testConnection(id: number): Promise<{ success: boolean; message: string }> {
    const response = await api.post(`/cloud/providers/${id}/test`);
    return response.data;
  }
}

export default new CloudService();
