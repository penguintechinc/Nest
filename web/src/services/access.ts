import api from './api';
import { TemporaryAccess, PaginatedResponse } from '../types/server';

class AccessService {
  async getTemporaryAccess(page = 1, per_page = 20): Promise<PaginatedResponse<TemporaryAccess>> {
    const response = await api.get('/temporary-access', { params: { page, per_page } });
    return response.data;
  }

  async grantAccess(data: Partial<TemporaryAccess>): Promise<TemporaryAccess> {
    const response = await api.post('/temporary-access', data);
    return response.data;
  }

  async revokeAccess(id: number): Promise<void> {
    await api.delete(`/temporary-access/${id}`);
  }
}

export default new AccessService();
