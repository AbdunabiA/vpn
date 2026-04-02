import api from './api';

export interface CheckoutSession {
  session_id: string;
  url: string;
}

// Creates a Stripe Checkout session for the given plan.
// Returns the session id and the hosted Stripe URL to open in the browser.
export async function createCheckoutSession(plan: string): Promise<CheckoutSession> {
  const {data} = await api.post<{data: CheckoutSession}>('/subscription/checkout', {
    plan,
  });
  return data.data;
}

// Cancels the authenticated user's active subscription at period end.
export async function cancelSubscription(): Promise<void> {
  await api.post('/subscription/cancel');
}
