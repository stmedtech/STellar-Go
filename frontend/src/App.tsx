import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom';
import { ToastContainer } from 'react-toastify';
import 'react-toastify/dist/ReactToastify.css';
import Layout from './components/Layout';
import Dashboard from './pages/Dashboard';
import Devices from './pages/Devices';
import Compute from './pages/Compute';
import ProxyPage from './pages/Proxy';
import PolicyPage from './pages/Policy';

function App() {
  return (
    <BrowserRouter>
      <Layout>
        <Routes>
          <Route path="/" element={<Navigate to="/app" replace />} />
          <Route path="/app" element={<Dashboard />} />
          <Route path="/app/devices" element={<Devices />} />
          <Route path="/app/compute" element={<Compute />} />
          <Route path="/app/proxy" element={<ProxyPage />} />
          <Route path="/app/policy" element={<PolicyPage />} />
        </Routes>
      </Layout>
      <ToastContainer
        position="top-right"
        autoClose={5000}
        hideProgressBar={false}
        newestOnTop={false}
        closeOnClick
        rtl={false}
        pauseOnFocusLoss
        draggable
        pauseOnHover
        theme="light"
      />
    </BrowserRouter>
  );
}

export default App;
