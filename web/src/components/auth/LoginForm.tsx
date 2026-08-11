/**
 * Login page component
 * Uses LoginPageBuilder from @penguintechinc/react-libs per standards.
 * Multi-tenant: the tenant field is shown and defaults to DEFAULT_TENANT.
 * API keys are never entered here — machine callers authenticate via the
 * tenant + API-key API route, not the interactive login page.
 */

import React from 'react';
import { useNavigate } from 'react-router-dom';
import { LoginPageBuilder } from '@penguintechinc/react-libs';
import type { LoginResponse } from '@penguintechinc/react-libs';
import useAuthStore from '../../stores/authStore';

/** Tenant used when a deployment has no explicit tenant configured. */
const DEFAULT_TENANT = 'nest';

const LoginForm: React.FC = () => {
  const navigate = useNavigate();
  const setUser = useAuthStore((state) => state.setUser);

  const handleSuccess = (response: LoginResponse) => {
    if (response.token) {
      localStorage.setItem('auth_token', response.token);
    }

    // The auth service is authoritative for tenant — persist what it returns,
    // never what was typed. Falls back to the default deployment tenant.
    const { tenant } = response as LoginResponse & { tenant?: string };
    localStorage.setItem('nest_tenant', tenant ?? DEFAULT_TENANT);

    if (response.user) {
      setUser({
        id: response.user.id,
        email: response.user.email,
        name: response.user.name ?? response.user.email,
      });
    }
    navigate('/dashboard');
  };

  return (
    <LoginPageBuilder
      api={{ loginUrl: '/api/v1/auth/login' }}
      branding={{
        appName: 'NEST',
        logo: '/nest-logo.png',
        logoHeight: 300,
        tagline: 'Data Manager',
        githubRepo: 'penguintechinc/nest',
      }}
      tenantField={{
        show: true,
        label: 'Tenant',
        placeholder: DEFAULT_TENANT,
        defaultValue: DEFAULT_TENANT,
        helpText: 'Leave as the default unless your organization uses a dedicated tenant.',
      }}
      onSuccess={handleSuccess}
      gdpr={{ enabled: true, privacyPolicyUrl: '/privacy' }}
    />
  );
};

export default LoginForm;
