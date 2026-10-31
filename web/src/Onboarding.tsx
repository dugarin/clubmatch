import { useState } from 'react';
import { api, ApiError, ProfileSelf } from './api';
import { TagInput } from './TagInput';

interface Props {
  initial?: ProfileSelf;
  onDone: (p: ProfileSelf) => void;
}

const CONSENT_VERSION = 'v1';

export function Onboarding({ initial, onDone }: Props) {
  const [firstName, setFirstName] = useState(initial?.first_name ?? '');
  const [lastName, setLastName] = useState(initial?.last_name ?? '');
  const [skills, setSkills] = useState<string[]>(initial?.skills ?? []);
  const [projects, setProjects] = useState((initial?.projects ?? []).join('\n'));
  const [profInterests, setProfInterests] = useState<string[]>(initial?.professional_interests ?? []);
  const [persInterests, setPersInterests] = useState<string[]>(initial?.personal_interests ?? []);
  const [location, setLocation] = useState(initial?.location ?? '');
  const [format, setFormat] = useState(initial?.meeting_format ?? '');
  const [consent, setConsent] = useState(false);
  const [error, setError] = useState('');
  const editing = !!initial;

  const submit = async () => {
    setError('');
    const body: Record<string, unknown> = {
      first_name: firstName,
      last_name: lastName,
      skills,
      projects: projects.split('\n').map((s) => s.trim()).filter(Boolean),
      professional_interests: profInterests,
      personal_interests: persInterests,
      location,
      meeting_format: format,
    };
    try {
      const saved = editing
        ? await api.patchProfile(body)
        : await api.createProfile({ ...body, consent_version: CONSENT_VERSION });
      onDone(saved);
    } catch (e) {
      setError(e instanceof ApiError ? `${e.status}: ${e.message}` : 'Network error');
    }
  };

  return (
    <div>
      <h2>{editing ? 'My profile' : 'Welcome to ClubMatch'}</h2>
      <label>First name *</label>
      <input value={firstName} onChange={(e) => setFirstName(e.target.value)} />
      <label>Last name</label>
      <input value={lastName} onChange={(e) => setLastName(e.target.value)} />
      <TagInput label="Skills *" values={skills} onChange={setSkills} />
      <label>Projects (one per line)</label>
      <textarea value={projects} onChange={(e) => setProjects(e.target.value)} rows={3} />
      <TagInput label="Professional interests" values={profInterests} onChange={setProfInterests} />
      <TagInput label="Personal interests" values={persInterests} onChange={setPersInterests} />
      <label>Location (city)</label>
      <input value={location} onChange={(e) => setLocation(e.target.value)} />
      <label>Meeting format *</label>
      <select value={format} onChange={(e) => setFormat(e.target.value)}>
        <option value="">—</option>
        <option value="online">Online</option>
        <option value="offline">Offline</option>
        <option value="any">Any</option>
      </select>
      {!editing && (
        <label style={{ fontWeight: 400 }}>
          <input
            type="checkbox"
            style={{ width: 'auto' }}
            checked={consent}
            onChange={(e) => setConsent(e.target.checked)}
          />{' '}
          I agree to the processing of my personal data and to sharing my Telegram contact upon mutual interest
        </label>
      )}
      {error && <p className="error">{error}</p>}
      <button className="primary" onClick={() => void submit()} disabled={!editing && !consent}>
        {editing ? 'Save' : 'Create profile'}
      </button>
    </div>
  );
}
