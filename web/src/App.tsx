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
