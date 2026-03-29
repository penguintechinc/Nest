import api from './api';
import { ManagedDatabase, PaginatedResponse } from '../types/server';

class DatabaseService {
  async getDatabases(page = 1, per_page = 20): Promise<PaginatedResponse<ManagedDatabase>> {
    const response = await api.get('/managed-databases', { params: { page, per_page } });
    return response.data;
  }

  async getDatabase(id: number): Promise<ManagedDatabase> {
    const response = await api.get(`/managed-databases/${id}`);
    return response.data;
  }

  async createDatabase(data: Partial<ManagedDatabase>): Promise<ManagedDatabase> {
    const response = await api.post('/managed-databases', data);
    return response.data;
  }

  async updateDatabase(id: number, data: Partial<ManagedDatabase>): Promise<ManagedDatabase> {
    const response = await api.put(`/managed-databases/${id}`, data);
    return response.data;
  }

  async deleteDatabase(id: number): Promise<void> {
    await api.delete(`/managed-databases/${id}`);
  }
}

export default new DatabaseService();
