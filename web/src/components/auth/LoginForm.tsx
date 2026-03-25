/**
 * Login page component
 * Uses LoginPageBuilder from @penguintechinc/react-libs per standards
 */

import React from 'react';
import { useNavigate } from 'react-router-dom';
import { LoginPageBuilder } from '@penguintechinc/react-libs';
import type { LoginResponse } from '@penguintechinc/react-libs';
import useAuthStore from '../../stores/authStore';

const LoginForm: React.FC = () => {
  const navigate = useNavigate();
  const setUser = useAuthStore((state) => state.setUser);

  const handleSuccess = (response: LoginResponse) => {
    if (response.token) {
      localStorage.setItem('auth_token', response.token);
    }
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
      onSuccess={handleSuccess}
      gdpr={{ enabled: true, privacyPolicyUrl: '/privacy' }}
    />
  );
};

export default LoginForm;
