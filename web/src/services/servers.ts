import api from './api';
import { DatabaseServer, PaginatedResponse } from '../types/server';

class ServerService {
  async getServers(page = 1, per_page = 20): Promise<PaginatedResponse<DatabaseServer>> {
    const response = await api.get('/database-servers', { params: { page, per_page } });
    return response.data;
  }

  async getServer(id: number): Promise<DatabaseServer> {
    const response = await api.get(`/database-servers/${id}`);
    return response.data;
  }

  async createServer(data: Partial<DatabaseServer>): Promise<DatabaseServer> {
    const response = await api.post('/database-servers', data);
    return response.data;
  }

  async updateServer(id: number, data: Partial<DatabaseServer>): Promise<DatabaseServer> {
    const response = await api.put(`/database-servers/${id}`, data);
    return response.data;
  }

  async deleteServer(id: number): Promise<void> {
    await api.delete(`/database-servers/${id}`);
  }
}

export default new ServerService();
