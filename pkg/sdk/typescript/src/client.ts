/** Nest TypeScript SDK client */
export interface DataResourceSpec {
  name: string;
  type: string;
  class: string;
  tenant: string;
  labels?: Record<string, string>;
}

export interface DataResource {
  id: string;
  name: string;
  type: string;
  class: string;
  tenant: string;
  status: string;
  endpoint?: string;
}

export interface DatabaseSpec {
  name: string;
  type: string;
  class: string;
  tenant: string;
}

export interface Database {
  id: string;
  name: string;
  type: string;
  class: string;
  tenant: string;
  status: string;
  endpoint?: string;
}

export class NestClient {
  private baseUrl: string;
  private token: string;

  constructor(baseUrl: string, token: string) {
    this.baseUrl = baseUrl.replace(/\/$/, '');
    this.token = token;
  }

  private async request<T>(method: string, path: string, body?: unknown): Promise<T> {
    const resp = await fetch(`${this.baseUrl}${path}`, {
      method,
      headers: {
        'Authorization': `Bearer ${this.token}`,
        'Content-Type': 'application/json',
      },
      body: body != null ? JSON.stringify(body) : undefined,
    });
    if (!resp.ok) {
      const err = await resp.json().catch(() => ({}));
      throw new Error(`Nest API ${resp.status}: ${(err as Record<string, string>).error ?? resp.statusText}`);
    }
    return resp.json() as Promise<T>;
  }

  readonly dataResources = {
    list: (tenant: string) =>
      this.request<{ dataresources: DataResource[] }>('GET', `/api/v1/tenants/${tenant}/dataresources`)
        .then(r => r.dataresources),

    get: (tenant: string, name: string) =>
      this.request<{ dataresource: DataResource }>('GET', `/api/v1/tenants/${tenant}/dataresources/${name}`)
        .then(r => r.dataresource),

    create: (tenant: string, spec: DataResourceSpec) =>
      this.request<void>('POST', `/api/v1/tenants/${tenant}/dataresources`, spec),

    delete: (tenant: string, name: string) =>
      this.request<void>('DELETE', `/api/v1/tenants/${tenant}/dataresources/${name}`),
  };

  readonly databases = {
    list: (tenant: string) =>
      this.request<{ databases: Database[] }>('GET', `/api/v1/tenants/${tenant}/databases`)
        .then(r => r.databases),

    create: (tenant: string, spec: DatabaseSpec) =>
      this.request<void>('POST', `/api/v1/tenants/${tenant}/databases`, spec),
  };
}
