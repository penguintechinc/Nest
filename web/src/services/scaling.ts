import api from './api';
import { ScalingPolicy, PaginatedResponse } from '../types/server';

class ScalingService {
  async getPolicies(page = 1, per_page = 20): Promise<PaginatedResponse<ScalingPolicy>> {
    const response = await api.get('/scaling/policies', { params: { page, per_page } });
    return response.data;
  }

  async createPolicy(data: Partial<ScalingPolicy>): Promise<ScalingPolicy> {
    const response = await api.post('/scaling/policies', data);
    return response.data;
  }

  async updatePolicy(id: number, data: Partial<ScalingPolicy>): Promise<ScalingPolicy> {
    const response = await api.put(`/scaling/policies/${id}`, data);
    return response.data;
  }

  async deletePolicy(id: number): Promise<void> {
    await api.delete(`/scaling/policies/${id}`);
  }

  async togglePolicy(id: number, enabled: boolean): Promise<ScalingPolicy> {
    const response = await api.patch(`/scaling/policies/${id}`, { enabled });
    return response.data;
  }
}

export default new ScalingService();
