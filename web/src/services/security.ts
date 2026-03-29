import api from './api';
import { SecurityRule, ThreatIntelEntry, BlockedDatabase, PaginatedResponse } from '../types/server';

class SecurityService {
  async getSecurityRules(page = 1, per_page = 20): Promise<PaginatedResponse<SecurityRule>> {
    const response = await api.get('/security-rules', { params: { page, per_page } });
    return response.data;
  }

  async createSecurityRule(data: Partial<SecurityRule>): Promise<SecurityRule> {
    const response = await api.post('/security-rules', data);
    return response.data;
  }

  async updateSecurityRule(id: number, data: Partial<SecurityRule>): Promise<SecurityRule> {
    const response = await api.put(`/security-rules/${id}`, data);
    return response.data;
  }

  async deleteSecurityRule(id: number): Promise<void> {
    await api.delete(`/security-rules/${id}`);
  }

  async getThreatIntel(page = 1, per_page = 20): Promise<PaginatedResponse<ThreatIntelEntry>> {
    const response = await api.get('/threat-intel', { params: { page, per_page } });
    return response.data;
  }

  async getBlockedDatabases(page = 1, per_page = 20): Promise<PaginatedResponse<BlockedDatabase>> {
    const response = await api.get('/blocked-databases', { params: { page, per_page } });
    return response.data;
  }

  async blockDatabase(data: Partial<BlockedDatabase>): Promise<BlockedDatabase> {
    const response = await api.post('/blocked-databases', data);
    return response.data;
  }

  async unblockDatabase(id: number): Promise<void> {
    await api.delete(`/blocked-databases/${id}`);
  }
}

export default new SecurityService();
