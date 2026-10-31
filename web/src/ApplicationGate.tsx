import { useState } from 'react';
import { api, ApiError, ApplicationStatus } from './api';

interface Props {
  status: ApplicationStatus;
  onRefresh: () => void;
}

export function ApplicationGate({ status, onRefresh }: Props) {
  const [error, setError] = useState('');
  const app = status.application;

  const submit = async () => {
    setError('');
    try {
      await api.submitApplication();
      onRefresh();
    } catch (e) {
      setError(e instanceof ApiError ? `${e.status}: ${e.message}` : 'Network error');
    }
  };

  return (
    <div>
      <h2>ClubMatch is a closed community</h2>
      {(!app || app.state === 'rejected') && (
        <>
          {app?.state === 'rejected' && (
            <p className="hint">Your application was not approved. You can apply again.</p>
          )}
          <p>Submit an application or join with an invite link.</p>
          {error && <p className="error">{error}</p>}
          <button className="primary" onClick={() => void submit()}>
            Apply
          </button>
        </>
      )}
      {app?.state === 'pending' && <p className="hint">Your application is pending review.</p>}
      {app?.state === 'approved' && <p className="hint">Approved! Reloading…</p>}
    </div>
  );
}
