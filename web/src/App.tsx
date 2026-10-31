import { useCallback, useEffect, useState } from 'react';
import { api, ApiError, ProfileSelf } from './api';
import { Onboarding } from './Onboarding';
import { Deck } from './Deck';
import { Search } from './Search';
import { Matches } from './Matches';
import { Profile } from './Profile';
import { ApplicationGate } from './ApplicationGate';

type Tab = 'deck' | 'search' | 'matches' | 'profile';

export default function App() {
  const [profile, setProfile] = useState<ProfileSelf | null>(null);
  const [needsOnboarding, setNeedsOnboarding] = useState(false);
  const [gate, setGate] = useState(false);
  const [tab, setTab] = useState<Tab>('deck');
  const [loading, setLoading] = useState(true);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      setProfile(await api.getProfile());
      setNeedsOnboarding(false);
      setGate(false);
    } catch (e) {
      if (e instanceof ApiError && e.status === 404) {
        setNeedsOnboarding(true);
      } else if (e instanceof ApiError && e.status === 403) {
        setGate(true);
      }
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  if (loading) return <div className="app">Loading…</div>;

  if (gate) {
    return (
      <div className="app">
        <GateLoader onResolved={() => void load()} />
      </div>
    );
  }

  if (needsOnboarding || !profile) {
    return (
      <div className="app">
        <Onboarding
          onDone={(p) => {
            setProfile(p);
            setNeedsOnboarding(false);
          }}
        />
      </div>
    );
  }

  return (
    <div className="app">
      <div className="tabs">
        {(['deck', 'search', 'matches', 'profile'] as Tab[]).map((t) => (
          <button key={t} className={tab === t ? 'active' : ''} onClick={() => setTab(t)}>
            {t[0].toUpperCase() + t.slice(1)}
          </button>
        ))}
      </div>
      {tab === 'deck' && <Deck />}
      {tab === 'search' && <Search />}
      {tab === 'matches' && <Matches />}
      {tab === 'profile' && <Profile profile={profile} onSaved={setProfile} onDeleted={() => void load()} />}
    </div>
  );
}

function GateLoader({ onResolved }: { onResolved: () => void }) {
  const [status, setStatus] = useState<Awaited<ReturnType<typeof api.applicationStatus>> | null>(null);
  useEffect(() => {
    void api.applicationStatus().then(setStatus).catch(() => setStatus(null));
  }, []);
  if (!status) return <p>Loading…</p>;
  if (status.access_state === 'approved') {
    onResolved();
    return null;
  }
  return (
    <ApplicationGate
      status={status}
      onRefresh={() => void api.applicationStatus().then(setStatus)}
    />
  );
}
