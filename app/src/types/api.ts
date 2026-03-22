// API response types for the Go Fiber backend

export interface ApiResponse<T> {
  data: T;
  error?: string;
}

export interface User {
  id: string;
  email_hash: string;
  subscription_tier: 'free' | 'premium' | 'ultimate';
  subscription_expires_at: string | null;
  created_at: string;
}

export interface AuthTokens {
  access_token: string;
  refresh_token: string;
  expires_in: number; // seconds until access token expires
}

export interface Server {
  id: string;
  hostname: string;
  ip_address: string;
  region: string;
  city: string;
  country: string;
  country_code: string;
  protocol: string;
  load_percent: number;
  is_active: boolean;
}

export interface ServerConfig {
  server_address: string;
  server_port: number;
  protocol: string;
  user_id: string;
  reality?: {
    public_key: string;
    short_id: string;
    server_name: string;
    fingerprint: string;
  };
}

export interface Subscription {
  id: string;
  plan: 'free' | 'premium' | 'ultimate';
  is_active: boolean;
  started_at: string;
  expires_at: string | null;
  max_devices: number;
}
