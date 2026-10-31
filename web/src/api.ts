declare global {
  interface Window {
    Telegram?: {
      WebApp?: {
        initData: string;
        ready: () => void;
        expand: () => void;
      };
    };
  }
}

export function initData(): string {
  const fromTg = window.Telegram?.WebApp?.initData;
  if (fromTg) return fromTg;
  const q = new URLSearchParams(window.location.search);
  return q.get('initData') ?? '';
}

export interface ProfileSelf {
  first_name: string;
  last_name?: string;
  skills: string[];
  projects: string[];
  professional_interests: string[];
  personal_interests: string[];
  location?: string;
  meeting_format: string;
  is_searchable: boolean;
  bot_linked: boolean;
  updated_at: string;
}

export interface Card {
  id: string;
  skills: string[];
  projects: string[];
  professional_interests: string[];
  personal_interests: string[];
  location?: string;
  meeting_format: string;
  my_evaluation?: string;
  first_name?: string;
  last_name?: string;
  telegram_username?: string | null;
  telegram_user_id?: number | null;
}

export interface MatchItem {
  match_id: string;
  state: string;
  card: Card | null;
  created_at: string;
}

export interface ApplicationStatus {
  access_state: string;
  application: { id: string; state: string; created_at: string } | null;
}

export class ApiError extends Error {
  constructor(
    public status: number,
    message: string,
  ) {
    super(message);
  }
}

async function request<T>(path: string, options: RequestInit = {}): Promise<T> {
  const resp = await fetch(path, {
    ...options,
    headers: {
      'Content-Type': 'application/json',
      Authorization: `tma ${initData()}`,
      ...(options.headers ?? {}),
    },
  });
  if (!resp.ok) {
    const body = await resp.text();
    throw new ApiError(resp.status, body);
  }
  if (resp.status === 204) return undefined as T;
  return resp.json() as Promise<T>;
}

export const api = {
  getProfile: () => request<ProfileSelf>('/api/v1/profiles/me'),
  createProfile: (body: unknown) =>
    request<ProfileSelf>('/api/v1/profiles', { method: 'POST', body: JSON.stringify(body) }),
  patchProfile: (body: unknown) =>
    request<ProfileSelf>('/api/v1/profiles/me', { method: 'PATCH', body: JSON.stringify(body) }),
  deleteProfile: () => request<void>('/api/v1/profiles/me', { method: 'DELETE' }),
  autocomplete: (q: string) =>
    request<{ id: string; canonical: string }[]>(`/api/v1/tags/autocomplete?q=${encodeURIComponent(q)}`),
  deck: () => request<Card[]>('/api/v1/deck'),
  evaluate: (targetId: string, value: string, source: string) =>
    request<{ match: boolean; card?: Card }>('/api/v1/evaluations', {
      method: 'POST',
      body: JSON.stringify({ target_id: targetId, value, source }),
    }),
  search: (q: string, location: string, meetingFormat: string, page: number) => {
    const params = new URLSearchParams({ q, page: String(page) });
    if (location) params.set('location', location);
    if (meetingFormat) params.set('meeting_format', meetingFormat);
    return request<{ items: Card[]; page: number; has_more: boolean; filters_applied: boolean }>(
      `/api/v1/search?${params}`,
    );
  },
  matches: () => request<MatchItem[]>('/api/v1/matches'),
  applicationStatus: () => request<ApplicationStatus>('/api/v1/applications/me'),
  submitApplication: () => request<void>('/api/v1/applications', { method: 'POST' }),
};
