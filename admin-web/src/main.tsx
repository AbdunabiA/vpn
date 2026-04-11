import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { BrowserRouter } from "react-router-dom";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { Toaster } from "sonner";

import App from "@/App";
import "./index.css";

// TanStack Query defaults: 30s stale, retry once. Admin data changes
// infrequently and we'd rather see a stale number than a loading spinner
// on every tab switch.
const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 30_000,
      retry: 1,
      refetchOnWindowFocus: false,
    },
  },
});

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      {/* The panel lives at the root of https://vpnadmin.mydayai.uz so
          no basename is needed — router paths match URL paths 1:1. */}
      <BrowserRouter>
        <App />
      </BrowserRouter>
      <Toaster richColors position="top-right" theme="dark" />
    </QueryClientProvider>
  </StrictMode>,
);
