import { Navigate, Route, Routes } from "react-router-dom";

import { AdminLayout } from "@/components/layout/AdminLayout";
import { Dashboard } from "@/pages/Dashboard";
import { Login } from "@/pages/Login";
import { Placeholder } from "@/pages/Placeholder";

export default function App() {
  return (
    <Routes>
      <Route path="/login" element={<Login />} />
      <Route element={<AdminLayout />}>
        <Route path="/" element={<Navigate to="/dashboard" replace />} />
        <Route path="/dashboard" element={<Dashboard />} />
        {/* Placeholders for routes that will be filled in during B-2/B-3/B-4.
            Keeping them wired to a single component means the sidebar links
            already work and the layout can be smoke-tested end-to-end. */}
        <Route
          path="/users"
          element={<Placeholder title="Users" subtitle="Coming in Phase B-2" />}
        />
        <Route
          path="/servers"
          element={
            <Placeholder title="Servers" subtitle="Coming in Phase B-3" />
          }
        />
        <Route
          path="/activity"
          element={
            <Placeholder title="Activity" subtitle="Coming in Phase B-3" />
          }
        />
        <Route
          path="/settings"
          element={
            <Placeholder title="Settings" subtitle="Coming in Phase B-4" />
          }
        />
      </Route>
      <Route path="*" element={<Navigate to="/dashboard" replace />} />
    </Routes>
  );
}
