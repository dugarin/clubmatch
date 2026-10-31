import { useState } from 'react';
import { api } from './api';

interface Props {
  label: string;
  values: string[];
  onChange: (values: string[]) => void;
}

export function TagInput({ label, values, onChange }: Props) {
  const [text, setText] = useState('');
  const [suggestions, setSuggestions] = useState<{ id: string; canonical: string }[]>([]);

  const updateSuggestions = async (q: string) => {
    setText(q);
    if (q.trim().length < 1) {
      setSuggestions([]);
      return;
    }
    try {
      setSuggestions(await api.autocomplete(q));
    } catch {
      setSuggestions([]);
    }
  };

  const add = (v: string) => {
    const t = v.trim();
    if (t && !values.includes(t)) onChange([...values, t]);
    setText('');
    setSuggestions([]);
  };

  return (
    <div>
      <label>{label}</label>
      <div className="tags">
        {values.map((v) => (
          <span key={v} className="tag">
            {v}{' '}
            <a onClick={() => onChange(values.filter((x) => x !== v))} style={{ cursor: 'pointer' }}>
              ×
            </a>
          </span>
        ))}
      </div>
      <input
        value={text}
        placeholder="Type or pick a tag"
        onChange={(e) => void updateSuggestions(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === 'Enter') {
            e.preventDefault();
            add(text);
          }
        }}
      />
      {suggestions.length > 0 && (
        <div className="suggest">
          {suggestions.map((s) => (
            <div key={s.id} onClick={() => add(s.canonical)}>
              {s.canonical}
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
