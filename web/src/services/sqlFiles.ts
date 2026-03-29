import api from './api';
import { SqlFile, PaginatedResponse } from '../types/server';

class SqlFileService {
  async getSqlFiles(page = 1, per_page = 20): Promise<PaginatedResponse<SqlFile>> {
    const response = await api.get('/sql-files', { params: { page, per_page } });
    return response.data;
  }

  async getSqlFile(id: number): Promise<SqlFile> {
    const response = await api.get(`/sql-files/${id}`);
    return response.data;
  }

  async createSqlFile(data: Partial<SqlFile>): Promise<SqlFile> {
    const response = await api.post('/sql-files', data);
    return response.data;
  }

  async reviewSqlFile(id: number, action: 'approve' | 'reject'): Promise<SqlFile> {
    const response = await api.post(`/sql-files/${id}/review`, { action });
    return response.data;
  }

  async executeSqlFile(id: number): Promise<SqlFile> {
    const response = await api.post(`/sql-files/${id}/execute`);
    return response.data;
  }

  async deleteSqlFile(id: number): Promise<void> {
    await api.delete(`/sql-files/${id}`);
  }
}

export default new SqlFileService();
