import { useEffect } from 'react';
import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom';
import { AppConsoleVersion } from '@penguintechinc/react-libs';
import useAuthStore from './stores/authStore';
import ProtectedRoute from './components/auth/ProtectedRoute';
import AppLayout from './components/layout/AppLayout';
import LoginForm from './components/auth/LoginForm';
import Dashboard from './pages/Dashboard';
import Resources from './pages/Resources';
import Teams from './pages/Teams';
import Servers from './pages/Servers';
import Databases from './pages/Databases';
import SqlFiles from './pages/SqlFiles';
import SecurityRules from './pages/SecurityRules';
import ThreatIntel from './pages/ThreatIntel';
import BlockedDatabases from './pages/BlockedDatabases';
import CloudProviders from './pages/CloudProviders';
import ScalingPolicies from './pages/ScalingPolicies';
import TemporaryAccess from './pages/TemporaryAccess';

function App() {
  const checkAuth = useAuthStore((state) => state.checkAuth);

  useEffect(() => {
    checkAuth();
  }, [checkAuth]);

  return (
    <BrowserRouter>
      <AppConsoleVersion
        appName="NEST"
        webuiVersion={import.meta.env.VITE_VERSION || '0.0.0'}
        webuiBuildEpoch={Number(import.meta.env.VITE_BUILD_TIME) || 0}
        environment={import.meta.env.MODE}
        apiStatusUrl="/api/v1/status"
        metadata={{
          'API URL': import.meta.env.VITE_API_URL || '(relative)',
        }}
      />
      <Routes>
        {/* Public routes */}
        <Route path="/login" element={<LoginForm />} />

        {/* Protected routes */}
        <Route
          path="/*"
          element={
            <ProtectedRoute>
              <AppLayout>
                <Routes>
                  <Route path="/dashboard" element={<Dashboard />} />
                  <Route path="/resources" element={<Resources />} />
                  <Route path="/teams" element={<Teams />} />
                  <Route path="/servers" element={<Servers />} />
                  <Route path="/databases" element={<Databases />} />
                  <Route path="/sql-files" element={<SqlFiles />} />
                  <Route path="/security-rules" element={<SecurityRules />} />
                  <Route path="/threat-intel" element={<ThreatIntel />} />
                  <Route path="/blocked-databases" element={<BlockedDatabases />} />
                  <Route path="/cloud-providers" element={<CloudProviders />} />
                  <Route path="/scaling" element={<ScalingPolicies />} />
                  <Route path="/temporary-access" element={<TemporaryAccess />} />
                  <Route path="/" element={<Navigate to="/dashboard" replace />} />
                </Routes>
              </AppLayout>
            </ProtectedRoute>
          }
        />
      </Routes>
    </BrowserRouter>
  );
}

export default App;
